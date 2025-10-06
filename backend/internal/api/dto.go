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
	ID                    uint      `json:"id"`
	Domain                string    `json:"domain"`
	TrademarkScore        int       `json:"trademark_score"`
	TrademarkType         string    `json:"trademark_type"`
	MatchedTrademark      string    `json:"matched_trademark"`
	TrademarkConfidence   float64   `json:"trademark_confidence"`
	ViceScore             int       `json:"vice_score"`
	ViceCategories        []string  `json:"vice_categories"`
	ViceConfidence        float64   `json:"vice_confidence"`
	OverallRecommendation string    `json:"overall_recommendation"`
	Confidence            float64   `json:"confidence"`
	CreatedAt             time.Time `json:"created_at"`
	Explanation           string    `json:"explanation"`
	CommercialOverride    bool      `json:"commercial_override"`
	CommercialSource      string    `json:"commercial_source"`
	CommercialSimilarity  float64   `json:"commercial_similarity"`
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
	return EvaluationDTO{
		ID:                    e.ID,
		Domain:                e.Domain,
		TrademarkScore:        e.TrademarkScore,
		TrademarkType:         e.TrademarkType,
		MatchedTrademark:      e.MatchedTrademark,
		TrademarkConfidence:   round2(e.TrademarkConfidence),
		ViceScore:             e.ViceScore,
		ViceCategories:        e.ViceCategories(),
		ViceConfidence:        round2(e.ViceConfidence),
		OverallRecommendation: e.OverallRecommendation,
		Confidence:            round2(minFloat(e.TrademarkConfidence, e.ViceConfidence)),
		CreatedAt:             e.CreatedAt,
		Explanation:           strings.TrimSpace(e.Explanation),
		CommercialOverride:    e.CommercialOverride,
		CommercialSource:      e.CommercialSource,
		CommercialSimilarity:  round2(e.CommercialSimilarity),
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
