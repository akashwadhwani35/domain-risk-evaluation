package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"domain-risk-eval/backend/internal/ai"
	"domain-risk-eval/backend/internal/api"
	"domain-risk-eval/backend/internal/usp"
)

func main() {
	baseDir, err := os.Getwd()
	if err != nil {
		logrus.Fatalf("determine working directory: %v", err)
	}

	dataDir := filepath.Join(baseDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		logrus.Fatalf("create data directory: %v", err)
	}

	defaultXML := filepath.Join(baseDir, "..", "apc250917.xml")
	defaultDomains := filepath.Join(baseDir, "..", "Test domains.csv")
	defaultCommercial := filepath.Join(baseDir, "..", "bquxjob_40fe6a70_1995182bb6e.csv")

	aiCfg := ai.Config{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   os.Getenv("OPENAI_MODEL"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
	}
	if temp := os.Getenv("OPENAI_TEMPERATURE"); temp != "" {
		if v, err := strconv.ParseFloat(temp, 64); err == nil {
			aiCfg.Temperature = v
		}
	}
	if maxTokens := os.Getenv("OPENAI_MAX_TOKENS"); maxTokens != "" {
		if v, err := strconv.Atoi(maxTokens); err == nil {
			aiCfg.MaxTokens = v
		}
	}

	usptoCfg := usp.Config{}
	if timeout := os.Getenv("USPTO_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			usptoCfg.Timeout = d
		}
	}
	if ttl := os.Getenv("USPTO_CACHE_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil {
			usptoCfg.CacheTTL = d
		}
	}
	if rows := os.Getenv("USPTO_ROWS"); rows != "" {
		if v, err := strconv.Atoi(rows); err == nil {
			usptoCfg.Rows = v
		}
	}

	commercialPath := defaultCommercial
	if envCommercial := strings.TrimSpace(os.Getenv("COMMERCIAL_SALES_PATH")); envCommercial != "" {
		commercialPath = envCommercial
	}

	popularLimit := 500000
	if v := strings.TrimSpace(os.Getenv("POPULAR_MARK_LIMIT")); v != "" {
		if val, err := strconv.Atoi(v); err == nil && val > 0 {
			popularLimit = val
		}
	}
	marksLimit := 500000
	if v := strings.TrimSpace(os.Getenv("MARKS_LIMIT")); v != "" {
		if val, err := strconv.Atoi(v); err == nil && val > 0 {
			marksLimit = val
		}
	}
	popularMinCount := 2
	if v := strings.TrimSpace(os.Getenv("POPULAR_MARK_MIN_COUNT")); v != "" {
		if val, err := strconv.Atoi(v); err == nil && val > 0 {
			popularMinCount = val
		}
	}

	disableAI := strings.EqualFold(strings.TrimSpace(os.Getenv("DISABLE_AI")), "true")

	cfg := api.Config{
		DBPath:             filepath.Join(dataDir, "domain-risk.db"),
		SeedsPath:          filepath.Join(baseDir, "internal", "scoring", "fanciful_seed.json"),
		ViceTermsPath:      filepath.Join(baseDir, "internal", "scoring", "vice_terms.json"),
		DefaultXMLPath:     defaultXML,
		DefaultDomainsPath: defaultDomains,
		CommercialSales:    commercialPath,
		AllowedOrigins: []string{
			"http://localhost:1000",
			"http://127.0.0.1:1000",
			"https://domain-risk-frontend.onrender.com",
		},
		AIConfig:        aiCfg,
		USPTOConfig:     usptoCfg,
		DisableAI:       disableAI,
		PopularLimit:    popularLimit,
		PopularMinCount: popularMinCount,
		MarksLimit:      marksLimit,
	}

	if override := strings.TrimSpace(os.Getenv("DOMAIN_RISK_DB_PATH")); override != "" {
		cfg.DBPath = override
	}

	server, err := api.NewServer(cfg)
	if err != nil {
		logrus.Fatalf("create server: %v", err)
	}

	router, err := server.Router()
	if err != nil {
		logrus.Fatalf("configure router: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "2000"
	}

	logrus.Infof("starting domain-risk-eval backend on :%s", port)
	if err := router.Run(":" + port); err != nil {
		logrus.Fatalf("server exited: %v", err)
	}
}
