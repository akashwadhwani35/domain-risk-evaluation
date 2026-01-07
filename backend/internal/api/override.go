package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"domain-risk-eval/backend/internal/ai"
	"domain-risk-eval/backend/internal/store"
)

// handleCreateOverride creates a new override for an evaluation.
// POST /api/evaluations/:id/override
func (s *Server) handleCreateOverride(c *gin.Context) {
	evaluationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, err)
		return
	}

	var req OverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logrus.WithError(err).WithField("evaluation_id", evaluationID).Warn("override: invalid request body")
		s.renderError(c, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	logrus.WithFields(logrus.Fields{
		"evaluation_id": evaluationID,
		"tm_rec":        req.OverrideTrademarkRecommendation,
		"vice_rec":      req.OverrideViceRecommendation,
		"overridden_by": req.OverriddenBy,
		"reason":        req.Reason,
	}).Info("override: received request")

	// Validate and normalize trademark recommendation if provided
	req.OverrideTrademarkRecommendation = strings.ToUpper(strings.TrimSpace(req.OverrideTrademarkRecommendation))
	if req.OverrideTrademarkRecommendation != "" {
		switch req.OverrideTrademarkRecommendation {
		case "YES_RISK", "NO_RISK", "POTENTIAL_RISK":
			// Valid
		default:
			s.renderError(c, http.StatusBadRequest, errors.New("invalid trademark recommendation: must be YES_RISK, NO_RISK, or POTENTIAL_RISK"))
			return
		}
	}

	// Validate and normalize vice recommendation if provided
	req.OverrideViceRecommendation = strings.ToUpper(strings.TrimSpace(req.OverrideViceRecommendation))
	if req.OverrideViceRecommendation != "" {
		switch req.OverrideViceRecommendation {
		case "YES_RISK", "NO_RISK", "POTENTIAL_RISK":
			// Valid
		default:
			s.renderError(c, http.StatusBadRequest, errors.New("invalid vice recommendation: must be YES_RISK, NO_RISK, or POTENTIAL_RISK"))
			return
		}
	}

	// Require at least one recommendation to be overridden
	if req.OverrideTrademarkRecommendation == "" && req.OverrideViceRecommendation == "" {
		logrus.WithField("evaluation_id", evaluationID).Warn("override: no recommendations provided")
		s.renderError(c, http.StatusBadRequest, errors.New("at least one of trademark or vice recommendation must be provided"))
		return
	}

	// Get the current evaluation
	evaluation, err := s.db.GetEvaluationByID(evaluationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.renderError(c, http.StatusNotFound, fmt.Errorf("evaluation %d not found", evaluationID))
		} else {
			s.renderError(c, http.StatusInternalServerError, err)
		}
		return
	}

	// Derive original recommendations if not stored
	origTmRec := evaluation.TrademarkRecommendation
	if origTmRec == "" {
		origTmRec = store.ScoreToRecommendation(evaluation.TrademarkScore)
	}
	origViceRec := evaluation.ViceRecommendation
	if origViceRec == "" {
		origViceRec = store.ScoreToRecommendation(evaluation.ViceScore)
	}

	// If user didn't override one, keep the original
	overrideTmRec := req.OverrideTrademarkRecommendation
	if overrideTmRec == "" {
		overrideTmRec = origTmRec
	}
	overrideViceRec := req.OverrideViceRecommendation
	if overrideViceRec == "" {
		overrideViceRec = origViceRec
	}

	// Derive overall recommendation from the two
	overrideOverall := "NO_RISK"
	if overrideTmRec == "YES_RISK" || overrideViceRec == "YES_RISK" {
		overrideOverall = "YES_RISK"
	} else if overrideTmRec == "POTENTIAL_RISK" || overrideViceRec == "POTENTIAL_RISK" {
		overrideOverall = "POTENTIAL_RISK"
	}

	// Create the override record with original values snapshot
	override := &store.EvaluationOverride{
		EvaluationID:                    evaluationID,
		DomainNormalized:                evaluation.DomainNormalized,
		OriginalTrademarkScore:          evaluation.TrademarkScore,
		OriginalTrademarkRecommendation: origTmRec,
		OriginalViceScore:               evaluation.ViceScore,
		OriginalViceRecommendation:      origViceRec,
		OriginalRecommendation:          evaluation.OverallRecommendation,
		OriginalExplanation:             evaluation.Explanation,
		OverrideTrademarkRecommendation: overrideTmRec,
		OverrideViceRecommendation:      overrideViceRec,
		OverrideRecommendation:          overrideOverall,
		OverrideExplanation:             strings.TrimSpace(req.OverrideExplanation),
		OverriddenBy:                    strings.TrimSpace(req.OverriddenBy),
		Reason:                          strings.TrimSpace(req.Reason),
		FeedbackApplied:                 false,
	}

	// Create the override and apply it to the evaluation
	if err := s.db.CreateOverride(override); err != nil {
		s.renderError(c, http.StatusInternalServerError, fmt.Errorf("create override: %w", err))
		return
	}

	if err := s.db.ApplyOverrideToEvaluation(evaluationID, override); err != nil {
		s.renderError(c, http.StatusInternalServerError, fmt.Errorf("apply override: %w", err))
		return
	}

	// Generate embedding for AI learning asynchronously
	if s.embeddingClient != nil && s.embeddingClient.Enabled() {
		go s.generateFeedbackEmbedding(override, evaluation)
	}

	logrus.WithFields(logrus.Fields{
		"evaluation_id": evaluationID,
		"domain":        evaluation.Domain,
		"overridden_by": override.OverriddenBy,
		"tm_from":       origTmRec,
		"tm_to":         overrideTmRec,
		"vice_from":     origViceRec,
		"vice_to":       overrideViceRec,
	}).Info("evaluation override created")

	c.JSON(http.StatusCreated, OverrideFromModel(*override))
}

// generateFeedbackEmbedding creates a feedback embedding for AI learning.
func (s *Server) generateFeedbackEmbedding(override *store.EvaluationOverride, evaluation *store.Evaluation) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Extract second level label from domain
	secondLevel := extractSecondLevel(evaluation.Domain)

	// Build the text to embed using recommendations
	explanation := override.OverrideExplanation
	if explanation == "" {
		explanation = override.Reason
	}
	feedbackText := ai.BuildFeedbackTextWithRecs(
		evaluation.Domain,
		secondLevel,
		override.OverrideTrademarkRecommendation,
		override.OverrideViceRecommendation,
		explanation,
	)

	// Generate embedding
	embedding, err := s.embeddingClient.GenerateEmbedding(ctx, feedbackText)
	if err != nil {
		logrus.WithError(err).WithField("override_id", override.ID).Warn("failed to generate feedback embedding")
		return
	}

	// Store the embedding
	feedbackEmb := &store.FeedbackEmbedding{
		OverrideID:                       override.ID,
		DomainNormalized:                 evaluation.DomainNormalized,
		SecondLevelLabel:                 secondLevel,
		EmbeddingText:                    feedbackText,
		CorrectedTrademarkRecommendation: override.OverrideTrademarkRecommendation,
		CorrectedViceRecommendation:      override.OverrideViceRecommendation,
		CorrectedRecommendation:          override.OverrideRecommendation,
		CorrectedExplanation:             explanation,
	}
	feedbackEmb.SetEmbedding(embedding)

	if err := s.db.CreateFeedbackEmbedding(feedbackEmb); err != nil {
		logrus.WithError(err).WithField("override_id", override.ID).Warn("failed to store feedback embedding")
		return
	}

	// Update override to mark feedback as applied
	override.FeedbackApplied = true
	override.EmbeddingID = &feedbackEmb.ID
	if err := s.db.UpdateOverrideFeedbackStatus(override.ID, feedbackEmb.ID); err != nil {
		logrus.WithError(err).WithField("override_id", override.ID).Warn("failed to update override feedback status")
	}

	logrus.WithFields(logrus.Fields{
		"override_id":  override.ID,
		"embedding_id": feedbackEmb.ID,
		"domain":       evaluation.Domain,
	}).Info("feedback embedding created for AI learning")
}

// extractSecondLevel extracts the second-level domain label.
func extractSecondLevel(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	// Remove protocol if present
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	// Remove path if present
	if idx := strings.Index(domain, "/"); idx >= 0 {
		domain = domain[:idx]
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return domain
	}
	return parts[len(parts)-2]
}

// handleGetEvaluationHistory returns an evaluation with its override history.
// GET /api/evaluations/:id/history
func (s *Server) handleGetEvaluationHistory(c *gin.Context) {
	evaluationID, err := parseUintParam(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, err)
		return
	}

	evaluation, err := s.db.GetEvaluationByID(evaluationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.renderError(c, http.StatusNotFound, fmt.Errorf("evaluation %d not found", evaluationID))
		} else {
			s.renderError(c, http.StatusInternalServerError, err)
		}
		return
	}

	overrides, err := s.db.GetOverridesByEvaluation(evaluationID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	overrideDTOs := make([]OverrideDTO, len(overrides))
	for i, o := range overrides {
		overrideDTOs[i] = OverrideFromModel(o)
	}

	c.JSON(http.StatusOK, OverrideHistoryResponse{
		Evaluation: FromModel(*evaluation),
		Overrides:  overrideDTOs,
	})
}

// handleListOverrides returns a paginated list of all overrides.
// GET /api/overrides
func (s *Server) handleListOverrides(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 0 {
		page = 0
	}
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if pageSize <= 0 {
		pageSize = 25
	}
	offset := page * pageSize

	user := strings.TrimSpace(c.Query("user"))

	overrides, total, err := s.db.ListAllOverrides(offset, pageSize, user)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	overrideDTOs := make([]OverrideDTO, len(overrides))
	for i, o := range overrides {
		overrideDTOs[i] = OverrideFromModel(o)
	}

	c.JSON(http.StatusOK, OverridesResponse{Items: overrideDTOs, Total: total})
}

// handleListFeedback returns a paginated list of feedback embeddings.
// GET /api/feedback
func (s *Server) handleListFeedback(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 0 {
		page = 0
	}
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if pageSize <= 0 {
		pageSize = 25
	}
	offset := page * pageSize

	embeddings, total, err := s.db.ListFeedbackEmbeddings(offset, pageSize)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, err)
		return
	}

	feedbackDTOs := make([]FeedbackDTO, len(embeddings))
	for i, f := range embeddings {
		feedbackDTOs[i] = FeedbackFromModel(f)
	}

	c.JSON(http.StatusOK, FeedbacksResponse{Items: feedbackDTOs, Total: total})
}

// handleGetFeedbackStats returns AI learning analytics.
// GET /api/feedback/stats
func (s *Server) handleGetFeedbackStats(c *gin.Context) {
	var batchID uint
	if value := c.Query("batch_id"); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			batchID = uint(parsed)
		}
	}

	stats, err := s.db.GetFeedbackStats(batchID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, fmt.Errorf("get feedback stats: %w", err))
		return
	}

	// Get total feedback embeddings
	_, totalEmbeddings, err := s.db.ListFeedbackEmbeddings(0, 0)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, fmt.Errorf("count embeddings: %w", err))
		return
	}

	// Calculate feedback impact score (how often feedback is being retrieved and used)
	feedbackImpact := 0.0
	if totalEmbeddings > 0 {
		allEmbeddings, err := s.db.GetAllEmbeddings()
		if err == nil {
			totalRetrievals := 0
			for _, e := range allEmbeddings {
				totalRetrievals += e.RetrievalCount
			}
			if totalRetrievals > 0 {
				feedbackImpact = float64(totalRetrievals) / float64(totalEmbeddings)
			}
		}
	}

	response := FeedbackStatsResponse{
		TotalOverrides:          stats.TotalOverrides,
		TotalFeedbackEmbeddings: totalEmbeddings,
		OverrideRatePercent:     stats.OverrideRate,
		FeedbackImpactScore:     round2(feedbackImpact),
		AIAccuracy: AIAccuracyMetrics{
			TotalEvaluations:       stats.TotalEvaluations,
			OverriddenEvaluations:  stats.TotalOverrides,
			AccuracyPercent:        round2(stats.AccuracyPercent),
			BlockToAllowRate:       round2(stats.BlockToAllowRate),
			AllowToBlockRate:       round2(stats.AllowToBlockRate),
			AverageScoreAdjustment: round2(stats.AvgScoreAdjustment),
		},
		CorrectionPatterns:    convertCorrectionPatterns(stats.CorrectionPatterns),
		UserStats:             convertUserStats(stats.UserStats),
		ConfidenceCalibration: convertCalibrationBuckets(stats.ConfidenceBuckets),
		OverridesOverTime:     convertTimeSeriesPoints(stats.OverridesOverTime),
		AccuracyOverTime:      convertTimeSeriesPoints(stats.AccuracyOverTime),
	}

	c.JSON(http.StatusOK, response)
}

func convertCorrectionPatterns(patterns []store.CorrectionPattern) []CorrectionPattern {
	result := make([]CorrectionPattern, len(patterns))
	for i, p := range patterns {
		result[i] = CorrectionPattern{
			FromRecommendation: p.FromRecommendation,
			ToRecommendation:   p.ToRecommendation,
			Count:              p.Count,
			Percentage:         round2(p.Percentage),
		}
	}
	return result
}

func convertUserStats(stats []store.UserOverrideStat) []UserOverrideStats {
	result := make([]UserOverrideStats, len(stats))
	for i, s := range stats {
		result[i] = UserOverrideStats{
			User:             s.User,
			TotalOverrides:   s.TotalOverrides,
			MostCommonChange: s.MostCommonChange,
			LastOverrideAt:   s.LastOverrideAt,
		}
	}
	return result
}

func convertCalibrationBuckets(buckets []store.ConfidenceBucket) []ConfidenceCalibrationBucket {
	result := make([]ConfidenceCalibrationBucket, len(buckets))
	for i, b := range buckets {
		result[i] = ConfidenceCalibrationBucket{
			ConfidenceMin:   b.ConfidenceMin,
			ConfidenceMax:   b.ConfidenceMax,
			TotalCount:      b.TotalCount,
			OverriddenCount: b.OverriddenCount,
			AccuracyPercent: round2(b.AccuracyPercent),
		}
	}
	return result
}

func convertTimeSeriesPoints(points []store.TimeSeriesPoint) []TimeSeriesPoint {
	result := make([]TimeSeriesPoint, len(points))
	for i, p := range points {
		result[i] = TimeSeriesPoint{
			Date:  p.Date,
			Value: round2(p.Value),
		}
	}
	return result
}
