package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"domain-risk-eval/backend/internal/scoring"
	"domain-risk-eval/backend/internal/store"
	xmlparser "domain-risk-eval/backend/internal/xml"
)

const (
	defaultDatasetURL = "https://api.uspto.gov/api/v1/datasets/products/trtyrap"
)

func main() {
	var (
		dbPath      = flag.String("db", filepath.FromSlash("backend/data/domain-risk.db"), "Path to SQLite database")
		xmlPaths    multiFlag
		xmlDirPaths multiFlag
		seedPath    = flag.String("seed", filepath.FromSlash("internal/scoring/fanciful_seed.json"), "Path to fanciful seed JSON")
		limit       = flag.Int("limit", 500000, "Maximum number of popular marks to keep")
		minCount    = flag.Int("min-count", 2, "Minimum occurrences for a mark to be considered popular")
		outputPath  = flag.String("output", "", "Optional path to write JSON array of popular tokens")
		refreshOnly = flag.Bool("refresh", false, "Only refresh aggregates without ingesting XML")
		datasetURL  = flag.String("dataset-url", "", "USPTO dataset endpoint (defaults to trtyrap)")
		datasetKey  = flag.String("dataset-key", "", "USPTO dataset API key (env USPTO_DATASET_KEY)")
		fromDate    = flag.String("from", "", "Dataset start date YYYY-MM-DD")
		toDate      = flag.String("to", "", "Dataset end date YYYY-MM-DD")
	)
	flag.Var(&xmlPaths, "xml", "USPTO bulk XML or ZIP file (repeatable)")
	flag.Var(&xmlDirPaths, "xml-dir", "Directory containing USPTO ZIP files (repeatable)")
	flag.Parse()

	loadEnvDefaults(datasetURL, datasetKey, fromDate, toDate)

	db, err := store.Open(*dbPath, true)
	if err != nil {
		logrus.Fatalf("open database: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			logrus.WithError(cerr).Warn("close database")
		}
	}()

	downloadList := make([]string, 0, len(xmlPaths))
	seen := make(map[string]struct{})

	addFile := func(path string) {
		cleaned := filepath.Clean(path)
		if cleaned == "" {
			return
		}
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		downloadList = append(downloadList, cleaned)
	}

	for _, p := range xmlPaths {
		addFile(p)
	}

	for _, dir := range xmlDirPaths {
		dir = filepath.Clean(dir)
		if dir == "" {
			continue
		}
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				logrus.WithError(err).WithField("path", path).Warn("walking xml dir")
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), ".zip") {
				addFile(path)
			}
			return nil
		})
	}

	if !*refreshOnly {
		urlTrimmed := strings.TrimSpace(*datasetURL)
		keyTrimmed := strings.TrimSpace(*datasetKey)
		if urlTrimmed != "" {
			files, err := fetchDatasetIndex(urlTrimmed, keyTrimmed, *fromDate, *toDate)
			if err != nil {
				if len(downloadList) == 0 {
					logrus.Fatalf("fetch dataset index: %v", err)
				}
				logrus.WithError(err).Warn("skipping dataset fetch; continuing with provided files")
			} else {
				for _, f := range files {
					dest, dlErr := downloadDatasetFile(f, keyTrimmed)
					if dlErr != nil {
						if len(downloadList) == 0 {
							logrus.Fatalf("download %s: %v", f.FileName, dlErr)
						}
						logrus.WithError(dlErr).WithField("file", f.FileName).Warn("skipping dataset file")
						continue
					}
					downloadList = append(downloadList, dest)
				}
			}
		}
	}

	if !*refreshOnly && len(downloadList) > 0 {
		decider, err := scoring.NewFancifulDecider(*seedPath)
		if err != nil {
			logrus.Fatalf("fanciful decider: %v", err)
		}
		for _, path := range downloadList {
			start := time.Now()
			logrus.WithField("file", path).Info("ingesting USPTO bulk data")
			ingested, err := xmlparser.Ingest(xmlparser.IngestOptions{
				Path:    path,
				DB:      db,
				Decider: decider,
				Progress: func(count int) {
					if count%50000 == 0 {
						logrus.WithField("file", path).WithField("marks", count).Info("ingest progress")
					}
				},
			})
			if err != nil {
				logrus.Fatalf("ingest %s: %v", path, err)
			}
			logrus.WithFields(logrus.Fields{
				"file":     path,
				"marks":    ingested,
				"duration": time.Since(start).Round(time.Second),
			}).Info("ingest complete")
		}
	}

	logrus.WithFields(logrus.Fields{
		"limit":     *limit,
		"min_count": *minCount,
	}).Info("building popular mark aggregates")

	popular, err := db.PopularMarks(*limit, *minCount)
	if err != nil {
		logrus.Fatalf("aggregate popular marks: %v", err)
	}
	if err := db.ReplacePopularMarks(popular); err != nil {
		logrus.Fatalf("persist popular marks: %v", err)
	}

	set := make(map[string]struct{}, len(popular))
	tokens := make([]string, 0, len(popular))
	for _, row := range popular {
		normalized := sanitize(row.Normalized)
		if normalized == "" {
			continue
		}
		if _, exists := set[normalized]; exists {
			continue
		}
		set[normalized] = struct{}{}
		tokens = append(tokens, normalized)
	}

	scoring.SetPopularTokens(set)
	logrus.WithField("popular_tokens", len(tokens)).Info("popular mark aggregation complete")

	if *outputPath != "" {
		if err := writeTokens(*outputPath, tokens); err != nil {
			logrus.Fatalf("write tokens: %v", err)
		}
		logrus.WithField("path", *outputPath).Info("popular tokens written to file")
	}
}

func loadEnvDefaults(datasetURL, datasetKey, fromDate, toDate *string) {
	if strings.TrimSpace(*datasetURL) == "" {
		if v := strings.TrimSpace(os.Getenv("USPTO_DATASET_URL")); v != "" {
			*datasetURL = v
		} else {
			*datasetURL = defaultDatasetURL
		}
	}
	if strings.TrimSpace(*datasetKey) == "" {
		*datasetKey = strings.TrimSpace(os.Getenv("USPTO_DATASET_KEY"))
	}
	if strings.TrimSpace(*fromDate) == "" {
		*fromDate = strings.TrimSpace(os.Getenv("USPTO_DATASET_FROM"))
	}
	if strings.TrimSpace(*toDate) == "" {
		*toDate = strings.TrimSpace(os.Getenv("USPTO_DATASET_TO"))
	}
}

func fetchDatasetIndex(baseURL, apiKey, fromDate, toDate string) ([]datasetFile, error) {
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	query := reqURL.Query()
	if strings.TrimSpace(fromDate) != "" {
		query.Set("fileDataFromDate", fromDate)
	}
	if strings.TrimSpace(toDate) != "" {
		query.Set("fileDataToDate", toDate)
	}
	query.Set("includeFiles", "true")
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("dataset request failed: %s", strings.TrimSpace(string(body)))
	}

	var decoded datasetResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}

	files := decoded.Files()
	if len(files) == 0 {
		return nil, errors.New("no dataset files found in response")
	}
	logrus.WithFields(logrus.Fields{
		"dataset": req.URL.String(),
		"files":   len(files),
	}).Info("dataset index retrieved")
	return files, nil
}

func downloadDatasetFile(file datasetFile, apiKey string) (string, error) {
	if file.FileURL == "" {
		return "", errors.New("missing file url")
	}
	req, err := http.NewRequest(http.MethodGet, file.FileURL, nil)
	if err != nil {
		return "", err
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	client := &http.Client{Timeout: 5 * time.Minute}

	logrus.WithField("url", file.FileURL).Info("downloading dataset file")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("download failed: %s", strings.TrimSpace(string(body)))
	}

	tmp, err := os.CreateTemp("", "uspto-*.zip")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}

	fileInfo, statErr := tmp.Stat()
	size := int64(0)
	if statErr == nil {
		size = fileInfo.Size()
	}
	logrus.WithFields(logrus.Fields{
		"file": tmp.Name(),
		"size": size,
	}).Info("dataset file downloaded")
	return tmp.Name(), nil
}

func writeTokens(path string, tokens []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(tokens)
}

func sanitize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

type datasetFile struct {
	FileName string `json:"fileName"`
	FileURL  string `json:"fileUrl"`
}

type datasetResponse struct {
	ProductFiles []datasetFile `json:"productFiles"`
	Result       struct {
		ProductFiles []datasetFile `json:"productFiles"`
	} `json:"result"`
	Response struct {
		Docs []struct {
			FileName string `json:"fileName"`
			FileURL  string `json:"fileLocation"`
		} `json:"docs"`
	} `json:"response"`
}

func (r datasetResponse) Files() []datasetFile {
	files := make([]datasetFile, 0)
	appendFile := func(f datasetFile) {
		if strings.TrimSpace(f.FileURL) == "" {
			return
		}
		if f.FileName == "" {
			f.FileName = filepath.Base(f.FileURL)
		}
		files = append(files, f)
	}

	for _, f := range r.ProductFiles {
		appendFile(f)
	}
	for _, f := range r.Result.ProductFiles {
		appendFile(f)
	}
	for _, doc := range r.Response.Docs {
		appendFile(datasetFile{FileName: doc.FileName, FileURL: doc.FileURL})
	}
	return files
}
