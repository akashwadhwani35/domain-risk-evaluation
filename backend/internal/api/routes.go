package api

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"domain-risk-eval/backend/internal/ai"
	"domain-risk-eval/backend/internal/commercial"
	"domain-risk-eval/backend/internal/match"
	"domain-risk-eval/backend/internal/scoring"
	"domain-risk-eval/backend/internal/store"
	"domain-risk-eval/backend/internal/usp"
)

// Config defines server dependencies.
type Config struct {
	DBPath             string
	SeedsPath          string
	ViceTermsPath      string
	DefaultXMLPath     string
	DefaultDomainsPath string
	CommercialSales    string
	AllowedOrigins     []string
	SilentDB           bool
	AIConfig           ai.Config
	USPTOConfig        usp.Config
	DisableAI          bool
	PopularLimit       int
	PopularMinCount    int
	MarksLimit         int
}

// Server wires HTTP handlers with persistence and scoring.
type Server struct {
	db              *store.Database
	seedPath        string
	vicePath        string
	defaultXMLPath  string
	defaultDomains  string
	viceScorer      *scoring.ViceScorer
	fancifulDecider *scoring.FancifulDecider
	allowedOrigins  []string
	explainer       ai.Explainer
	usptoClient     *usp.Client
	evalNotifier    *EvaluationNotifier
	jobMu           sync.Mutex
	activeJob       *evaluationJob
	commercial      *commercial.Service
	commercialPath  string
	popularLimit    int
	popularMinCount int
	marksLimit      int
	marksOnce       sync.Once
	marksCache      []store.Mark
	marksErr        error
}

const commercialMinPrice = 10000.0

// NewServer constructs the API server.
func NewServer(cfg Config) (*Server, error) {
	if cfg.DBPath == "" {
		return nil, errors.New("db path required")
	}
	db, err := store.Open(cfg.DBPath, cfg.SilentDB)
	if err != nil {
		return nil, err
	}

	seedPath := cfg.SeedsPath
	if seedPath == "" {
		seedPath = filepath.Join("internal", "scoring", "fanciful_seed.json")
	}
	vicePath := cfg.ViceTermsPath
	if vicePath == "" {
		vicePath = filepath.Join("internal", "scoring", "vice_terms.json")
	}

	decider, err := scoring.NewFancifulDecider(seedPath)
	if err != nil {
		return nil, fmt.Errorf("fanciful decider: %w", err)
	}
	viceScorer, err := scoring.NewViceScorer(vicePath)
	if err != nil {
		return nil, fmt.Errorf("vice scorer: %w", err)
	}

	var explainer ai.Explainer
	if cfg.DisableAI {
		logrus.Info("AI explainer disabled via configuration")
	} else {
		if client, err := ai.NewClient(cfg.AIConfig); err == nil {
			explainer = client
		} else if errors.Is(err, ai.ErrDisabled) {
			return nil, fmt.Errorf("ai explainer disabled: configure OpenAI credentials")
		} else {
			return nil, fmt.Errorf("ai client: %w", err)
		}
	}

	var usptoClient *usp.Client
	if strings.TrimSpace(cfg.USPTOConfig.APIKey) == "" {
		logrus.Info("USPTO lookup disabled - no API key configured")
	} else {
		client, err := usp.NewClient(cfg.USPTOConfig)
		if err != nil {
			return nil, fmt.Errorf("usp client: %w", err)
		}
		usptoClient = client
		logrus.WithFields(logrus.Fields{
			"rows":    cfg.USPTOConfig.Rows,
			"ttl":     cfg.USPTOConfig.CacheTTL,
			"timeout": cfg.USPTOConfig.Timeout,
		}).Info("USPTO lookup enabled")
	}

	server := &Server{
		db:              db,
		seedPath:        seedPath,
		vicePath:        vicePath,
		defaultXMLPath:  cfg.DefaultXMLPath,
		defaultDomains:  cfg.DefaultDomainsPath,
		viceScorer:      viceScorer,
		fancifulDecider: decider,
		allowedOrigins:  cfg.AllowedOrigins,
		explainer:       explainer,
		usptoClient:     usptoClient,
		evalNotifier:    NewEvaluationNotifier(),
		commercial:      commercial.NewService(db),
		popularLimit:    cfg.PopularLimit,
		popularMinCount: cfg.PopularMinCount,
		marksLimit:      cfg.MarksLimit,
	}

	if server.marksLimit <= 0 {
		server.marksLimit = 500000
	}

	if trimmed := strings.TrimSpace(cfg.CommercialSales); trimmed != "" {
		if err := server.loadCommercialSales(trimmed); err != nil {
			logrus.WithError(err).Warn("load commercial sales data")
		}
	}

	if cfg.PopularLimit > 0 {
		if count, err := scoring.LoadPopularTokensFromStore(db, cfg.PopularLimit); err != nil {
			logrus.WithError(err).Warn("load cached popular tokens")
		} else if count == 0 && cfg.PopularMinCount > 0 {
			if refreshed, aggErr := scoring.LoadPopularTokens(db, cfg.PopularLimit, cfg.PopularMinCount); aggErr != nil {
				logrus.WithError(aggErr).Warn("compute popular tokens")
			} else {
				logrus.WithField("popular_tokens", refreshed).Info("computed popular mark tokens")
			}
		} else {
			logrus.WithField("popular_tokens", count).Info("loaded popular mark tokens")
		}
	}

	return server, nil
}

// Router configures gin routes.
func (s *Server) Router() (*gin.Engine, error) {
	r := gin.Default()

	corsCfg := cors.DefaultConfig()
	corsCfg.AllowCredentials = true
	if len(s.allowedOrigins) == 0 {
		corsCfg.AllowAllOrigins = true
	} else {
		corsCfg.AllowOrigins = s.allowedOrigins
	}
	corsCfg.AllowHeaders = []string{"Origin", "Content-Type", "Accept"}
	corsCfg.AllowMethods = []string{"GET", "POST", "DELETE", "OPTIONS"}
	r.Use(cors.New(corsCfg))

	r.GET("/api/healthz", s.handleHealth)
	r.GET("/api/config", s.handleConfig)

	api := r.Group("/api")
	{
		api.GET("/batches", s.handleListBatches)
		api.GET("/batches/:id", s.handleGetBatch)
		api.GET("/batches/:id/results", s.handleBatchResults)
		api.GET("/requests/:id/status", s.handleRequestStatus)
		api.POST("/upload", s.handleUpload)
		api.POST("/evaluate", s.handleEvaluate)
		api.GET("/evaluate/status", s.handleEvaluateStatus)
		api.DELETE("/evaluate/:jobID", s.handleCancelEvaluate)
		api.GET("/evaluate/stream", s.handleEvaluateStream)
		api.GET("/results", s.handleResults)
		api.GET("/export.csv", s.handleExportCSV)
		api.GET("/export.json", s.handleExportJSON)
	}

	return r, nil
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleConfig(c *gin.Context) {
	tlds, err := s.listTLDs()
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	commercialRecords := 0
	if s.commercial != nil {
		commercialRecords = s.commercial.Count()
	}

	c.JSON(http.StatusOK, gin.H{
		"seed_path":                s.seedPath,
		"vice_terms_path":          s.vicePath,
		"tlds":                     tlds,
		"commercial_sales_records": commercialRecords,
	})
}
func (s *Server) loadTrademarkMarks() ([]store.Mark, error) {
	s.marksOnce.Do(func() {
		limit := s.marksLimit
		if limit <= 0 {
			limit = 500000
		}

		start := time.Now()
		logrus.WithFields(logrus.Fields{
			"marks_limit": limit,
		}).Info("loading trademark marks from store")
		marks, err := scoring.LoadMarks(s.db, limit)
		duration := time.Since(start)
		if err != nil {
			s.marksErr = err
			logrus.WithError(err).WithFields(logrus.Fields{
				"marks_limit": limit,
				"duration":    duration,
			}).Error("load trademark marks failed")
			return
		}
		s.marksCache = marks
		logrus.WithFields(logrus.Fields{
			"marks_loaded": len(marks),
			"marks_limit":  limit,
			"duration":     duration,
		}).Info("trademark marks cached")
	})

	if s.marksErr == nil {
		logrus.WithFields(logrus.Fields{
			"marks_cached": len(s.marksCache),
			"marks_limit":  s.marksLimit,
		}).Debug("trademark marks ready")
	}
	return s.marksCache, s.marksErr
}

func (s *Server) handleListBatches(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 0 {
		page = 0
	}
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if pageSize <= 0 {
		pageSize = 25
	}
	offset := page * pageSize

	rows, total, err := s.db.ListCSVBatches(offset, pageSize)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}
	dtos := make([]BatchDTO, 0, len(rows))
	for _, row := range rows {
		dtos = append(dtos, BatchFromModel(row))
	}
	c.JSON(http.StatusOK, BatchesResponse{Items: dtos, Total: total})
}

func (s *Server) handleGetBatch(c *gin.Context) {
	batchID, err := parseUintParam(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, err)
		return
	}

	batch, err := s.db.GetCSVBatch(batchID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.renderError(c, http.StatusNotFound, fmt.Errorf("batch %d not found", batchID))
		} else {
			s.renderError(c, http.StatusInternalServerError, err)
		}
		return
	}

	processed, err := s.db.CountBatchResults(batch.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}
	dto := BatchFromModel(*batch)
	dto.ProcessedDomains = processed
	c.JSON(http.StatusOK, dto)
}

func (s *Server) handleBatchResults(c *gin.Context) {
	batchID, err := parseUintParam(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, err)
		return
	}
	if _, err := s.db.GetCSVBatch(batchID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.renderError(c, http.StatusNotFound, fmt.Errorf("batch %d not found", batchID))
		} else {
			s.renderError(c, http.StatusInternalServerError, err)
		}
		return
	}
	s.renderResults(c, batchID)
}

func (s *Server) handleRequestStatus(c *gin.Context) {
	requestID, err := parseUintParam(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, err)
		return
	}

	request, err := s.db.GetBatchRequest(requestID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.renderError(c, http.StatusNotFound, fmt.Errorf("request %d not found", requestID))
		} else {
			s.renderError(c, http.StatusInternalServerError, err)
		}
		return
	}

	c.JSON(http.StatusOK, BatchRequestFromModel(*request))
}

func (s *Server) handleUpload(c *gin.Context) {
	batchName := strings.TrimSpace(c.PostForm("batch_name"))
	if batchName == "" {
		s.renderError(c, http.StatusBadRequest, errors.New("batch_name is required"))
		return
	}
	ownerName := strings.TrimSpace(c.PostForm("owner_name"))
	if ownerName == "" {
		s.renderError(c, http.StatusBadRequest, errors.New("owner_name is required"))
		return
	}

	fileHeader, err := c.FormFile("domains")
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, http.ErrMissingFile) {
			s.renderError(c, status, errors.New("domains csv file is required"))
		} else {
			s.renderError(c, status, err)
		}
		return
	}

	path, cleanup, err := saveFormFile(fileHeader)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}
	if cleanup != nil {
		defer cleanup()
	}

	parsed, err := parseDomainCSV(path)
	if err != nil {
		s.renderError(c, http.StatusBadRequest, err)
		return
	}
	if parsed.rowCount == 0 {
		s.renderError(c, http.StatusBadRequest, errors.New("no domains detected in csv"))
		return
	}

	marksCount, err := s.db.CountMarks()
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	existing, err := s.db.ExistingEvaluationKeys(parsed.uniqueNormalized)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}
	existingCount := len(existing)

	batch, err := s.db.CreateCSVBatch(batchName, ownerName, fileHeader.Filename)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	for _, domain := range parsed.domainModels {
		if err := s.db.SaveDomain(domain); err != nil {
			s.renderError(c, http.StatusInternalServerError, fmt.Errorf("save domain %s: %w", domain.Domain, err))
			return
		}
	}

	for i := range parsed.domainBatches {
		parsed.domainBatches[i].BatchID = batch.ID
	}

	if err := s.db.ReplaceDomainBatch(batch.ID, parsed.domainBatches); err != nil {
		s.renderError(c, http.StatusInternalServerError, fmt.Errorf("store batch domains: %w", err))
		return
	}

	processedCount, err := s.db.CountBatchResults(batch.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	if err := s.db.UpdateCSVBatchStats(
		batch.ID,
		parsed.rowCount,
		len(parsed.domainModels),
		existingCount,
		parsed.duplicateRows,
		processedCount,
	); err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, UploadResponse{
		BatchID:         batch.ID,
		BatchName:       batch.Name,
		Owner:           batch.Owner,
		RowCount:        parsed.rowCount,
		UniqueDomains:   len(parsed.domainModels),
		ExistingDomains: existingCount,
		DuplicateRows:   parsed.duplicateRows,
		Processed:       processedCount,
		MarksCount:      int(marksCount),
	})
}

func (s *Server) handleEvaluate(c *gin.Context) {
	var req EvaluateRequest
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			s.renderError(c, http.StatusBadRequest, err)
			return
		}
	}

	if req.BatchID == 0 {
		s.renderError(c, http.StatusBadRequest, errors.New("batch_id is required"))
		return
	}

	batch, err := s.db.GetCSVBatch(req.BatchID)
	if err != nil {
		s.renderError(c, http.StatusNotFound, fmt.Errorf("batch %d not found", req.BatchID))
		return
	}

	totalDomains, err := s.db.CountBatchDomains(batch.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}
	if totalDomains == 0 {
		s.renderError(c, http.StatusBadRequest, errors.New("batch has no domains to evaluate"))
		return
	}

	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.activeJob != nil {
		s.renderError(c, http.StatusConflict, errors.New("evaluation already running"))
		return
	}

	job, err := s.startEvaluation(req, batch, int64(totalDomains))
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	response := StartEvaluationResponse{
		JobID:     job.id,
		BatchID:   batch.ID,
		RequestID: job.requestID,
		Total:     job.total,
		StartedAt: job.startedAt,
	}
	c.JSON(http.StatusAccepted, response)
}

func (s *Server) handleCancelEvaluate(c *gin.Context) {
	jobID := strings.TrimSpace(c.Param("jobID"))
	if jobID == "" {
		s.renderError(c, http.StatusBadRequest, errors.New("job id required"))
		return
	}

	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.activeJob == nil {
		s.renderError(c, http.StatusNotFound, errors.New("no evaluation running"))
		return
	}
	if s.activeJob.id != jobID {
		s.renderError(c, http.StatusNotFound, errors.New("job not found"))
		return
	}

	s.activeJob.cancel()
	logrus.WithField("job", jobID).Info("evaluation cancellation requested")
	s.evalNotifier.Broadcast(EvaluationEvent{
		Type:      "progress",
		JobID:     s.activeJob.id,
		BatchID:   s.activeJob.batchID,
		Total:     s.activeJob.total,
		Processed: 0,
		Message:   "cancellation requested",
	})

	c.JSON(http.StatusAccepted, gin.H{"status": "cancelling"})
}

func (s *Server) handleEvaluateStatus(c *gin.Context) {
	s.jobMu.Lock()
	job := s.activeJob
	s.jobMu.Unlock()

	status := s.evalNotifier.LastStatus()

	resp := EvaluateStatusResponse{
		Running: job != nil,
	}

	if job != nil {
		resp.JobID = job.id
		resp.BatchID = job.batchID
		resp.RequestID = job.requestID
		resp.Total = job.total
	}

	if status != nil {
		resp.State = status.Type
		resp.Message = status.Message
		if status.Processed != 0 {
			resp.Processed = status.Processed
		}
		if status.Total != 0 {
			resp.Total = status.Total
		}
		if status.BatchID != 0 {
			resp.BatchID = status.BatchID
		}
		if status.Evaluation != nil {
			copyEval := *status.Evaluation
			resp.LastEvaluation = &copyEval
		}
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleEvaluateStream(c *gin.Context) {
	upgrader := websocket.Upgrader{
		HandshakeTimeout:  5 * time.Second,
		EnableCompression: true,
		CheckOrigin: func(r *http.Request) bool {
			if len(s.allowedOrigins) == 0 {
				return true
			}
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			for _, allowed := range s.allowedOrigins {
				if strings.EqualFold(origin, allowed) {
					return true
				}
			}
			return false
		},
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logrus.WithError(err).Warn("upgrade websocket")
		return
	}

	client := s.evalNotifier.Register(conn)
	logrus.WithField("remote", conn.RemoteAddr().String()).Info("evaluation websocket connected")
	defer s.evalNotifier.Unregister(client)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if !websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logrus.WithField("remote", conn.RemoteAddr().String()).Info("evaluation websocket closed")
			} else {
				logrus.WithError(err).Warn("evaluation websocket unexpected close")
			}
			break
		}
	}
}

func (s *Server) handleResults(c *gin.Context) {
	batchID := uint(0)
	if value := strings.TrimSpace(firstNonEmpty(c.Query("batch_id"), c.Query("batchId"))); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			s.renderError(c, http.StatusBadRequest, fmt.Errorf("invalid batch_id: %s", value))
			return
		}
		batchID = uint(parsed)
	}
	s.renderResults(c, batchID)
}

func (s *Server) renderResults(c *gin.Context, batchID uint) {
	query := strings.TrimSpace(c.Query("q"))
	minScore, _ := strconv.Atoi(c.Query("minScore"))
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 0 {
		page = 0
	}
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if pageSize <= 0 {
		pageSize = 100
	}
	offset := page * pageSize

	minViceScore, _ := strconv.Atoi(c.Query("minViceScore"))
	tld := strings.TrimSpace(c.Query("tld"))
	recommendation := strings.TrimSpace(c.Query("recommendation"))
	sort := strings.TrimSpace(c.Query("sort"))

	rows, total, err := s.db.ListEvaluations(store.EvaluationQuery{
		Query:          query,
		MinTrademark:   minScore,
		MinVice:        minViceScore,
		TLD:            tld,
		Recommendation: recommendation,
		Sort:           sort,
		Offset:         offset,
		Limit:          pageSize,
		BatchID:        batchID,
	})
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}
	dtos := make([]EvaluationDTO, 0, len(rows))
	for _, row := range rows {
		dtos = append(dtos, FromModel(row))
	}
	c.JSON(http.StatusOK, EvaluateResponse{Items: dtos, Total: total})
}

func (s *Server) handleExportCSV(c *gin.Context) {
	batchID := uint(0)
	if value := strings.TrimSpace(firstNonEmpty(c.Query("batch_id"), c.Query("batchId"))); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			s.renderError(c, http.StatusBadRequest, fmt.Errorf("invalid batch_id: %s", value))
			return
		}
		batchID = uint(parsed)
	}

	rows, _, err := s.db.ListEvaluations(store.EvaluationQuery{Limit: -1, BatchID: batchID})
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	c.Header("Content-Disposition", "attachment; filename=domain-risk-export.csv")
	c.Header("Content-Type", "text/csv")

	writer := csv.NewWriter(c.Writer)
	headers := []string{"domain", "trademark_score", "trademark_type", "matched_trademark", "vice_score", "vice_categories", "overall_recommendation", "confidence", "ai_explanation", "commercial_override", "commercial_source", "commercial_similarity"}
	if err := writer.Write(headers); err != nil {
		return
	}
	for _, row := range rows {
		dto := FromModel(row)
		line := []string{
			dto.Domain,
			strconv.Itoa(dto.TrademarkScore),
			dto.TrademarkType,
			dto.MatchedTrademark,
			strconv.Itoa(dto.ViceScore),
			strings.Join(dto.ViceCategories, "|"),
			dto.OverallRecommendation,
			fmt.Sprintf("%.2f", dto.Confidence),
			dto.Explanation,
			strconv.FormatBool(dto.CommercialOverride),
			dto.CommercialSource,
			fmt.Sprintf("%.2f", dto.CommercialSimilarity),
		}
		if err := writer.Write(line); err != nil {
			return
		}
	}
	writer.Flush()
}

func (s *Server) handleExportJSON(c *gin.Context) {
	batchID := uint(0)
	if value := strings.TrimSpace(firstNonEmpty(c.Query("batch_id"), c.Query("batchId"))); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			s.renderError(c, http.StatusBadRequest, fmt.Errorf("invalid batch_id: %s", value))
			return
		}
		batchID = uint(parsed)
	}

	rows, _, err := s.db.ListEvaluations(store.EvaluationQuery{Limit: -1, BatchID: batchID})
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}
	dtos := make([]EvaluationDTO, 0, len(rows))
	for _, row := range rows {
		dtos = append(dtos, FromModel(row))
	}
	c.Header("Content-Disposition", "attachment; filename=domain-risk-export.json")
	c.JSON(http.StatusOK, dtos)
}

func (s *Server) lookupUSPTO(ctx context.Context, brandToken string, cache map[string]usp.LookupResult) (usp.LookupResult, bool) {
	if s.usptoClient == nil {
		return usp.LookupResult{}, false
	}
	key := strings.ToLower(strings.TrimSpace(brandToken))
	if key == "" {
		return usp.LookupResult{}, false
	}
	if cached, ok := cache[key]; ok {
		return cached, cached.Checked
	}
	result, err := s.usptoClient.LookupExact(ctx, key)
	if err != nil {
		logrus.WithError(err).Warn("usp lookup")
		cache[key] = usp.LookupResult{Term: key, Checked: false}
		return usp.LookupResult{}, false
	}
	cache[key] = result
	return result, result.Checked
}

func (s *Server) resolveTrademark(profile match.DomainProfile, hasLookup bool, lookup usp.LookupResult, fallback scoring.TrademarkResult) (scoring.TrademarkResult, []string) {
	closeMatches := make([]string, 0)
	sldToken := secondLevelToken(profile)

	if fallback.Score > 0 && fallback.MatchedTrademark != "" {
		closeMatches = append(closeMatches, fallback.MatchedTrademark)
		return fallback, uniqueStrings(closeMatches)
	}

	if hasLookup && lookup.Checked {
		for _, exact := range lookup.ExactMatches {
			if exact.Mark == "" {
				continue
			}
			closeMatches = append(closeMatches, exact.Mark)
			if cleanToken(exact.Mark) != sldToken {
				continue
			}
			isFanciful := false
			if s.fancifulDecider != nil {
				isFanciful = s.fancifulDecider.Decide(exact.Mark, exact.Classes, exact.Owner)
			}
			if isFanciful {
				return scoring.TrademarkResult{
					Score:            5,
					Type:             "fanciful",
					MatchedTrademark: exact.Mark,
					Confidence:       0.98,
				}, uniqueStrings(closeMatches)
			}
			if scoring.IsPopularToken(exact.Mark) {
				return scoring.TrademarkResult{
					Score:            2,
					Type:             "popular",
					MatchedTrademark: exact.Mark,
					Confidence:       0.75,
				}, uniqueStrings(closeMatches)
			}
			return scoring.TrademarkResult{
				Score:            0,
				Type:             "generic",
				MatchedTrademark: exact.Mark,
				Confidence:       0.4,
			}, uniqueStrings(closeMatches)
		}
		for _, sim := range lookup.Similar {
			if sim.Mark != "" {
				closeMatches = append(closeMatches, sim.Mark)
			}
		}
	}

	return scoring.TrademarkResult{Score: 0, Type: "none", Confidence: 0.4}, uniqueStrings(closeMatches)
}

func cleanToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	return replacer.Replace(value)
}

func secondLevelToken(profile match.DomainProfile) string {
	return cleanToken(secondLevelLabel(profile))
}

func secondLevelLabel(profile match.DomainProfile) string {
	host := strings.ToLower(strings.TrimSpace(profile.Host))
	if host == "" {
		return ""
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return parts[len(parts)-2]
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, item := range in {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func (s *Server) renderError(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"error": err.Error()})
}

func saveFormFile(header *multipart.FileHeader) (string, func(), error) {
	if header == nil {
		return "", nil, errors.New("file header is nil")
	}
	src, err := header.Open()
	if err != nil {
		return "", nil, err
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "upload-*"+filepath.Ext(header.Filename))
	if err != nil {
		return "", nil, err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	return tmp.Name(), cleanup, nil
}

type csvParseResult struct {
	domainModels     []*store.Domain
	domainBatches    []store.DomainBatch
	uniqueDomains    []string
	uniqueNormalized []string
	rowCount         int
	duplicateRows    int
}

func parseDomainCSV(path string) (*csvParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	var (
		domainCol       = -1
		headerProcessed bool
		uniqueMap       = make(map[string]*store.Domain)
		order           []string
		batches         []store.DomainBatch
		rowIndex        int
	)

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv: %w", err)
		}
		if len(record) == 0 {
			continue
		}

		if !headerProcessed {
			domainCol = detectDomainColumn(record)
			headerProcessed = true
			if domainCol >= 0 {
				continue // header row, move to next record
			}
			domainCol = 0
		}

		if domainCol < 0 || domainCol >= len(record) {
			domainCol = 0
		}

		value := strings.TrimSpace(record[domainCol])
		if value == "" {
			continue
		}
		value = strings.TrimPrefix(value, "\ufeff")
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		rowIndex++
		key := strings.ToLower(strings.TrimSpace(value))
		batches = append(batches, store.DomainBatch{Domain: value, DomainNormalized: key, RowIndex: rowIndex})

		if _, ok := uniqueMap[key]; !ok {
			profile := match.NormalizeDomain(value)
			domainModel := &store.Domain{
				Domain:           value,
				DomainNormalized: key,
				BrandToken:       profile.BrandToken,
			}
			tokens := append([]string{}, profile.Tokens...)
			tokens = append(tokens, profile.AltSplits...)
			domainModel.SetTokens(dedupe(tokens))
			uniqueMap[key] = domainModel
			order = append(order, key)
		}
	}

	uniqueModels := make([]*store.Domain, 0, len(order))
	uniqueDomains := make([]string, 0, len(order))
	uniqueNormalized := make([]string, 0, len(order))
	for _, key := range order {
		model := uniqueMap[key]
		if model == nil {
			continue
		}
		uniqueModels = append(uniqueModels, model)
		uniqueDomains = append(uniqueDomains, model.Domain)
		uniqueNormalized = append(uniqueNormalized, key)
	}

	duplicates := rowIndex - len(uniqueModels)
	if duplicates < 0 {
		duplicates = 0
	}

	return &csvParseResult{
		domainModels:     uniqueModels,
		domainBatches:    batches,
		uniqueDomains:    uniqueDomains,
		uniqueNormalized: uniqueNormalized,
		rowCount:         rowIndex,
		duplicateRows:    duplicates,
	}, nil
}

func detectDomainColumn(record []string) int {
	for idx, value := range record {
		normalized := strings.ToLower(strings.TrimSpace(value))
		switch normalized {
		case "domain", "domains", "url", "hostname", "host":
			return idx
		}
	}
	return -1
}

func (s *Server) loadCommercialSales(path string) error {
	if s.commercial == nil {
		s.commercial = commercial.NewService(s.db)
	}
	count, err := s.commercial.LoadFromCSV(path, commercialMinPrice)
	if err != nil {
		return err
	}
	s.commercialPath = path
	logrus.WithFields(logrus.Fields{
		"path":    path,
		"records": count,
	}).Info("commercial sales inventory loaded")
	return nil
}

func dedupe(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func parseUintParam(value string) (uint, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, errors.New("identifier is required")
	}
	parsed, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid identifier: %w", err)
	}
	if parsed == 0 {
		return 0, errors.New("identifier must be greater than zero")
	}
	return uint(parsed), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *Server) listTLDs() ([]string, error) {
	rows, _, err := s.db.ListDomains(0, 0)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		profile := match.NormalizeDomain(row.Domain)
		parts := strings.Split(strings.ToLower(profile.Host), ".")
		if len(parts) == 0 {
			continue
		}
		tld := strings.TrimSpace(parts[len(parts)-1])
		if tld == "" {
			continue
		}
		set[tld] = struct{}{}
	}
	tlds := make([]string, 0, len(set))
	for tld := range set {
		tlds = append(tlds, tld)
	}
	sort.Strings(tlds)
	return tlds, nil
}
