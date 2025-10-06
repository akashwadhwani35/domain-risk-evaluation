package xml

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"domain-risk-eval/backend/internal/store"
)

// FancifulDecider allows callers to influence fanciful determination while ingesting marks.
type FancifulDecider interface {
	Decide(markNormalized string, classes []string, owner string) bool
}

// IngestOptions configures the XML ingestion routine.
type IngestOptions struct {
	Path     string
	DB       *store.Database
	Decider  FancifulDecider
	Progress func(count int)
	Context  context.Context
}

// Ingest parses the USPTO XML (optionally zipped) and persists marks into the database.
func Ingest(opts IngestOptions) (int, error) {
	if opts.DB == nil {
		return 0, errors.New("db is required")
	}
	if opts.Path == "" {
		return 0, errors.New("path is required")
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	r, closer, err := openXML(opts.Path)
	if err != nil {
		return 0, err
	}
	defer closer()

	decoder := xml.NewDecoder(bufio.NewReader(r))
	count := 0

	for {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return count, nil
			}
			return count, fmt.Errorf("decode token: %w", err)
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "case-file" {
			continue
		}

		var cf caseFile
		if err := decoder.DecodeElement(&cf, &start); err != nil {
			return count, fmt.Errorf("decode case-file: %w", err)
		}

		markRecord := cf.toMark()
		if markRecord.Mark == "" {
			continue
		}

		markRecord.IsFanciful = decideFanciful(opts.Decider, markRecord.MarkNormalized, markRecord.Classes(), markRecord.Owner)
		if err := opts.DB.UpsertMark(markRecord); err != nil {
			return count, fmt.Errorf("upsert mark: %w", err)
		}
		count++
		if opts.Progress != nil && count%500 == 0 {
			opts.Progress(count)
		}
	}
}

func decideFanciful(decider FancifulDecider, markNormalized string, classes []string, owner string) bool {
	if decider != nil {
		return decider.Decide(markNormalized, classes, owner)
	}
	if len(markNormalized) >= 6 && len(classes) >= 2 {
		return true
	}
	return false
}

// openXML opens either a raw XML file or a ZIP containing one.
func openXML(path string) (io.Reader, func(), error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("%s is a directory", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".zip" {
		return openFromZip(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func openFromZip(path string) (io.Reader, func(), error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, err
	}
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			rc, err := f.Open()
			if err != nil {
				_ = zr.Close()
				return nil, nil, err
			}
			closer := func() {
				_ = rc.Close()
				_ = zr.Close()
			}
			return rc, closer, nil
		}
	}
	_ = zr.Close()
	return nil, nil, fmt.Errorf("no xml file found in %s", path)
}

type caseFile struct {
	SerialNumber       string              `xml:"serial-number"`
	RegistrationNumber string              `xml:"registration-number"`
	CaseFileHeader     caseFileHeader      `xml:"case-file-header"`
	Owners             caseFileOwners      `xml:"case-file-owners"`
	Classifications    caseClassifications `xml:"classifications"`
}

type caseFileHeader struct {
	MarkIdentification string `xml:"mark-identification"`
}

type caseFileOwners struct {
	Owners []caseFileOwner `xml:"case-file-owner"`
}

type caseFileOwner struct {
	PartyName string `xml:"party-name"`
}

type caseClassifications struct {
	Items []classification `xml:"classification"`
}

type classification struct {
	InternationalCodes []string `xml:"international-code"`
}

func (cf caseFile) toMark() *store.Mark {
	mark := strings.TrimSpace(cf.CaseFileHeader.MarkIdentification)
	mark = cleanString(mark)
	if mark == "" {
		return &store.Mark{}
	}
	owner := ""
	if len(cf.Owners.Owners) > 0 {
		owner = cleanString(cf.Owners.Owners[0].PartyName)
	}

	var classes []string
	for _, item := range cf.Classifications.Items {
		for _, code := range item.InternationalCodes {
			code = strings.TrimSpace(code)
			if code != "" {
				classes = append(classes, code)
			}
		}
	}

	normalized := strings.ToLower(mark)
	normalized = strings.Join(strings.Fields(normalized), " ")
	noSpaces := removeNonAlphaNum(normalized)

	m := &store.Mark{
		Serial:         strings.TrimSpace(cf.SerialNumber),
		Registration:   strings.TrimSpace(cf.RegistrationNumber),
		Mark:           mark,
		MarkNormalized: normalized,
		MarkNoSpaces:   noSpaces,
		Owner:          owner,
	}
	m.SetClasses(classes)
	return m
}

func cleanString(in string) string {
	return strings.TrimSpace(strings.ReplaceAll(in, "\n", " "))
}

func removeNonAlphaNum(in string) string {
	var b strings.Builder
	for _, r := range in {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
