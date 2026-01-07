package api

import (
	"encoding/json"
	"strings"
	"time"

	"domain-risk-eval/backend/internal/store"
)

// UploadResponse reports batch statistics after processing a CSV upload.
type UploadResponse struct {
	BatchID         uint   `json:"batch_id"`
	BatchName       string `json:"batch_name"`
	Owner           string `json:"owner"`
	RowCount        int    `json:"row_count"`
	UniqueDomains   int    `json:"unique_domains"`
	ExistingDomains int    `json:"existing_domains"`
	DuplicateRows   int    `json:"duplicate_rows"`
	Processed       int    `json:"processed_domains"`
	MarksCount      int    `json:"marks_count"`
}

// EvaluateRequest controls pagination for evaluation runs.
type EvaluateRequest struct {
	BatchID uint `json:"batch_id"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	Resume  bool `json:"resume"`
	Force   bool `json:"force"`
}

// SingleEvaluateRequest requests a one-off evaluation without a batch.
type SingleEvaluateRequest struct {
	Domain  string `json:"domain"`
	Persist bool   `json:"persist"`
}

// SingleEvaluateResponse returns a single evaluation payload.
type SingleEvaluateResponse struct {
	Evaluation EvaluationDTO `json:"evaluation"`
}

// EvaluateResponse holds evaluation items and totals.
type EvaluateResponse struct {
	Items []EvaluationDTO `json:"items"`
	Total int64           `json:"total"`
}

// StartEvaluationResponse describes the asynchronous evaluation kickoff payload.
type StartEvaluationResponse struct {
	JobID     string    `json:"job_id"`
	BatchID   uint      `json:"batch_id"`
	RequestID uint      `json:"request_id"`
	Total     int64     `json:"total"`
	StartedAt time.Time `json:"started_at"`
}

// EvaluationDTO is the API representation for a persisted evaluation.
type EvaluationDTO struct {
	ID     uint   `json:"id"`
	Domain string `json:"domain"`
	// Trademark classification
	TrademarkRecommendation string  `json:"trademark_recommendation"` // YES_RISK, POTENTIAL_RISK, NO_RISK
	TrademarkConfidence     float64 `json:"trademark_confidence"`
	TrademarkType           string  `json:"trademark_type"`
	MatchedTrademark        string  `json:"matched_trademark"`
	// Vice classification
	ViceRecommendation string   `json:"vice_recommendation"` // YES_RISK, POTENTIAL_RISK, NO_RISK
	ViceConfidence     float64  `json:"vice_confidence"`
	ViceCategories     []string `json:"vice_categories"`
	// Timestamps and metadata
	CreatedAt            time.Time  `json:"created_at"`
	Explanation          string     `json:"explanation"`
	CommercialOverride   bool       `json:"commercial_override"`
	CommercialSource     string     `json:"commercial_source"`
	CommercialSimilarity float64    `json:"commercial_similarity"`
	ManualOverride       bool       `json:"manual_override"`
	OverrideCount        int        `json:"override_count"`
	LastOverrideAt       *time.Time `json:"last_override_at"`
	FeedbackUsed         []uint     `json:"feedback_used"`
}

// BatchDTO represents metadata for an uploaded CSV dataset.
type BatchDTO struct {
	ID               uint       `json:"id"`
	Name             string     `json:"name"`
	Owner            string     `json:"owner"`
	OriginalFilename string     `json:"original_filename"`
	RowCount         int        `json:"row_count"`
	UniqueDomains    int        `json:"unique_domains"`
	ExistingDomains  int        `json:"existing_domains"`
	DuplicateRows    int        `json:"duplicate_rows"`
	ProcessedDomains int        `json:"processed_domains"`
	CreatedAt        time.Time  `json:"created_at"`
	LastEvaluatedAt  *time.Time `json:"last_evaluated_at"`
}

// BatchesResponse is the paginated response for CSV batches.
type BatchesResponse struct {
	Items []BatchDTO `json:"items"`
	Total int64      `json:"total"`
}

// BatchRequestDTO represents evaluation request tracking metadata.
type BatchRequestDTO struct {
	ID         uint       `json:"id"`
	BatchID    uint       `json:"batch_id"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	JobID      string     `json:"job_id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// FromModel converts a store.Evaluation into the DTO representation.
func FromModel(e store.Evaluation) EvaluationDTO {
	// Derive recommendations from scores if not set (for backward compatibility)
	tmRec := e.TrademarkRecommendation
	if tmRec == "" {
		tmRec = store.ScoreToRecommendation(e.TrademarkScore)
	}
	viceRec := e.ViceRecommendation
	if viceRec == "" {
		viceRec = store.ScoreToRecommendation(e.ViceScore)
	}

	return EvaluationDTO{
		ID:                      e.ID,
		Domain:                  e.Domain,
		TrademarkRecommendation: tmRec,
		TrademarkConfidence:     round2(e.TrademarkConfidence),
		TrademarkType:           e.TrademarkType,
		MatchedTrademark:        e.MatchedTrademark,
		ViceRecommendation:      viceRec,
		ViceConfidence:          round2(e.ViceConfidence),
		ViceCategories:          e.ViceCategories(),
		CreatedAt:               e.CreatedAt,
		Explanation:             strings.TrimSpace(e.Explanation),
		CommercialOverride:      e.CommercialOverride,
		CommercialSource:        e.CommercialSource,
		CommercialSimilarity:    round2(e.CommercialSimilarity),
		ManualOverride:          e.ManualOverride,
		OverrideCount:           e.OverrideCount,
		LastOverrideAt:          e.LastOverrideAt,
		FeedbackUsed:            e.FeedbackUsed(),
	}
}

// BatchFromModel converts a store.CSVBatch into a DTO.
func BatchFromModel(b store.CSVBatch) BatchDTO {
	return BatchDTO{
		ID:               b.ID,
		Name:             b.Name,
		Owner:            b.Owner,
		OriginalFilename: b.OriginalFilename,
		RowCount:         b.RowCount,
		UniqueDomains:    b.UniqueDomains,
		ExistingDomains:  b.ExistingDomains,
		DuplicateRows:    b.DuplicateRows,
		ProcessedDomains: b.ProcessedDomains,
		CreatedAt:        b.CreatedAt,
		LastEvaluatedAt:  b.LastEvaluatedAt,
	}
}

// BatchRequestFromModel converts a store.BatchRequest into a DTO.
func BatchRequestFromModel(r store.BatchRequest) BatchRequestDTO {
	return BatchRequestDTO{
		ID:         r.ID,
		BatchID:    r.BatchID,
		Type:       r.Type,
		Status:     r.Status,
		JobID:      r.JobID,
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
	}
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func minFloat(a, b float64) float64 {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

// MarshalJSON ensures deterministic vice ordering.
func (dto EvaluationDTO) MarshalJSON() ([]byte, error) {
	type Alias EvaluationDTO
	return json.Marshal(Alias(dto))
}

// EvaluateStatusResponse describes the state of the active evaluation job.
type EvaluateStatusResponse struct {
	Running        bool           `json:"running"`
	JobID          string         `json:"job_id"`
	BatchID        uint           `json:"batch_id"`
	RequestID      uint           `json:"request_id"`
	State          string         `json:"state"`
	Message        string         `json:"message"`
	Processed      int            `json:"processed"`
	Total          int64          `json:"total"`
	LastEvaluation *EvaluationDTO `json:"last_evaluation,omitempty"`
}

// OverrideRequest is the request body for creating an evaluation override.
type OverrideRequest struct {
	OverriddenBy string `json:"overridden_by" binding:"required"`
	Reason       string `json:"reason" binding:"required"`
	// Separate trademark and vice recommendations
	OverrideTrademarkRecommendation string `json:"override_trademark_recommendation"` // YES_RISK, POTENTIAL_RISK, NO_RISK
	OverrideViceRecommendation      string `json:"override_vice_recommendation"`      // YES_RISK, POTENTIAL_RISK, NO_RISK
	OverrideExplanation             string `json:"override_explanation"`
}

// OverrideDTO is the API representation of an evaluation override.
type OverrideDTO struct {
	ID           uint   `json:"id"`
	EvaluationID uint   `json:"evaluation_id"`
	Domain       string `json:"domain"`
	// Original values
	OriginalTrademarkRecommendation string `json:"original_trademark_recommendation"`
	OriginalViceRecommendation      string `json:"original_vice_recommendation"`
	OriginalExplanation             string `json:"original_explanation"`
	// Override values
	OverrideTrademarkRecommendation string `json:"override_trademark_recommendation"`
	OverrideViceRecommendation      string `json:"override_vice_recommendation"`
	OverrideExplanation             string `json:"override_explanation"`
	// Audit
	OverriddenBy    string    `json:"overridden_by"`
	Reason          string    `json:"reason"`
	FeedbackApplied bool      `json:"feedback_applied"`
	CreatedAt       time.Time `json:"created_at"`
}

// OverrideFromModel converts a store.EvaluationOverride into a DTO.
func OverrideFromModel(o store.EvaluationOverride) OverrideDTO {
	// Derive original recommendations from scores if not set (backward compat)
	origTmRec := o.OriginalTrademarkRecommendation
	if origTmRec == "" {
		origTmRec = store.ScoreToRecommendation(o.OriginalTrademarkScore)
	}
	origViceRec := o.OriginalViceRecommendation
	if origViceRec == "" {
		origViceRec = store.ScoreToRecommendation(o.OriginalViceScore)
	}

	return OverrideDTO{
		ID:                              o.ID,
		EvaluationID:                    o.EvaluationID,
		Domain:                          o.DomainNormalized,
		OriginalTrademarkRecommendation: origTmRec,
		OriginalViceRecommendation:      origViceRec,
		OriginalExplanation:             o.OriginalExplanation,
		OverrideTrademarkRecommendation: o.OverrideTrademarkRecommendation,
		OverrideViceRecommendation:      o.OverrideViceRecommendation,
		OverrideExplanation:             o.OverrideExplanation,
		OverriddenBy:                    o.OverriddenBy,
		Reason:                          o.Reason,
		FeedbackApplied:                 o.FeedbackApplied,
		CreatedAt:                       o.CreatedAt,
	}
}

// OverrideHistoryResponse contains an evaluation and its override history.
type OverrideHistoryResponse struct {
	Evaluation EvaluationDTO `json:"evaluation"`
	Overrides  []OverrideDTO `json:"overrides"`
}

// OverridesResponse is the paginated response for overrides.
type OverridesResponse struct {
	Items []OverrideDTO `json:"items"`
	Total int64         `json:"total"`
}

// FeedbackDTO is the API representation of a feedback embedding.
type FeedbackDTO struct {
	ID                               uint       `json:"id"`
	OverrideID                       uint       `json:"override_id"`
	Domain                           string     `json:"domain"`
	SecondLevelLabel                 string     `json:"second_level_label"`
	CorrectedTrademarkRecommendation string     `json:"corrected_trademark_recommendation"`
	CorrectedViceRecommendation      string     `json:"corrected_vice_recommendation"`
	CorrectedExplanation             string     `json:"corrected_explanation"`
	RetrievalCount                   int        `json:"retrieval_count"`
	LastRetrievedAt                  *time.Time `json:"last_retrieved_at"`
	CreatedAt                        time.Time  `json:"created_at"`
}

// FeedbackFromModel converts a store.FeedbackEmbedding into a DTO.
func FeedbackFromModel(f store.FeedbackEmbedding) FeedbackDTO {
	return FeedbackDTO{
		ID:                               f.ID,
		OverrideID:                       f.OverrideID,
		Domain:                           f.DomainNormalized,
		SecondLevelLabel:                 f.SecondLevelLabel,
		CorrectedTrademarkRecommendation: f.CorrectedTrademarkRecommendation,
		CorrectedViceRecommendation:      f.CorrectedViceRecommendation,
		CorrectedExplanation:             f.CorrectedExplanation,
		RetrievalCount:                   f.RetrievalCount,
		LastRetrievedAt:                  f.LastRetrievedAt,
		CreatedAt:                        f.CreatedAt,
	}
}

// FeedbacksResponse is the paginated response for feedback embeddings.
type FeedbacksResponse struct {
	Items []FeedbackDTO `json:"items"`
	Total int64         `json:"total"`
}

// AIAccuracyMetrics tracks AI performance statistics.
type AIAccuracyMetrics struct {
	TotalEvaluations       int64   `json:"total_evaluations"`
	OverriddenEvaluations  int64   `json:"overridden_evaluations"`
	AccuracyPercent        float64 `json:"accuracy_percent"`
	BlockToAllowRate       float64 `json:"block_to_allow_rate"`
	AllowToBlockRate       float64 `json:"allow_to_block_rate"`
	AverageScoreAdjustment float64 `json:"average_score_adjustment"`
}

// CorrectionPattern tracks how often the AI is corrected from one state to another.
type CorrectionPattern struct {
	FromRecommendation string `json:"from_recommendation"`
	ToRecommendation   string `json:"to_recommendation"`
	Count              int    `json:"count"`
	Percentage         float64 `json:"percentage"`
}

// UserOverrideStats tracks per-user override statistics.
type UserOverrideStats struct {
	User              string `json:"user"`
	TotalOverrides    int    `json:"total_overrides"`
	MostCommonChange  string `json:"most_common_change"`
	LastOverrideAt    *time.Time `json:"last_override_at"`
}

// ConfidenceCalibrationBucket tracks accuracy per confidence level.
type ConfidenceCalibrationBucket struct {
	ConfidenceMin   float64 `json:"confidence_min"`
	ConfidenceMax   float64 `json:"confidence_max"`
	TotalCount      int     `json:"total_count"`
	OverriddenCount int     `json:"overridden_count"`
	AccuracyPercent float64 `json:"accuracy_percent"`
}

// TimeSeriesPoint represents a data point on a time series chart.
type TimeSeriesPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// FeedbackStatsResponse contains full AI learning analytics.
type FeedbackStatsResponse struct {
	TotalOverrides          int64                         `json:"total_overrides"`
	TotalFeedbackEmbeddings int64                         `json:"total_feedback_embeddings"`
	OverrideRatePercent     float64                       `json:"override_rate_percent"`
	FeedbackImpactScore     float64                       `json:"feedback_impact_score"`
	AIAccuracy              AIAccuracyMetrics             `json:"ai_accuracy"`
	CorrectionPatterns      []CorrectionPattern           `json:"correction_patterns"`
	UserStats               []UserOverrideStats           `json:"user_stats"`
	ConfidenceCalibration   []ConfidenceCalibrationBucket `json:"confidence_calibration"`
	OverridesOverTime       []TimeSeriesPoint             `json:"overrides_over_time"`
	AccuracyOverTime        []TimeSeriesPoint             `json:"accuracy_over_time"`
}

// TrainingTermDTO represents an AI training term.
type TrainingTermDTO struct {
	ID             uint      `json:"id"`
	Term           string    `json:"term"`
	Classification string    `json:"classification"`
	CreatedAt      time.Time `json:"created_at"`
}

// TrainingTermsResponse contains training terms grouped by classification.
type TrainingTermsResponse struct {
	YesRisk []TrainingTermDTO `json:"yes_risk"`
	NoRisk  []TrainingTermDTO `json:"no_risk"`
}

// CreateTrainingTermRequest is the request body for adding a training term.
type CreateTrainingTermRequest struct {
	Term           string `json:"term" binding:"required"`
	Classification string `json:"classification" binding:"required"`
}
