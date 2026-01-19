package store

import (
	"encoding/json"
	"strings"
	"time"
)

// Mark represents a USPTO trademark entry persisted from the bulk XML feed.
type Mark struct {
	Serial         string `gorm:"primaryKey;size:32"`
	Registration   string `gorm:"size:32"`
	Mark           string `gorm:"size:256;index"`
	MarkNormalized string `gorm:"size:256;index"`
	MarkNoSpaces   string `gorm:"size:256;index"`
	Owner          string `gorm:"size:256"`
	ClassesJSON    string `gorm:"type:text"`
	IsFanciful     bool   `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// PopularMark stores aggregated mark usage counts for popularity scoring.
type PopularMark struct {
	Normalized string `gorm:"primaryKey;size:256"`
	Mark       string `gorm:"size:256"`
	Total      int    `gorm:"index"`
	UpdatedAt  time.Time
}

// SetClasses persists the class list as JSON.
func (m *Mark) SetClasses(classes []string) {
	if classes == nil {
		m.ClassesJSON = "[]"
		return
	}
	payload, _ := json.Marshal(classes)
	m.ClassesJSON = string(payload)
}

// Classes returns the unmarshalled class codes.
func (m *Mark) Classes() []string {
	if strings.TrimSpace(m.ClassesJSON) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(m.ClassesJSON), &out); err != nil {
		return nil
	}
	return out
}

// Domain represents a domain under evaluation.
type Domain struct {
	ID               uint   `gorm:"primaryKey"`
	Domain           string `gorm:"size:255;index"`
	DomainNormalized string `gorm:"size:255;uniqueIndex"`
	BrandToken       string `gorm:"size:255;index"`
	TokensJSON       string `gorm:"type:text"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SetTokens stores the heuristic token list.
func (d *Domain) SetTokens(tokens []string) {
	payload, _ := json.Marshal(tokens)
	d.TokensJSON = string(payload)
}

// Tokens reads the stored token list.
func (d *Domain) Tokens() []string {
	if strings.TrimSpace(d.TokensJSON) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(d.TokensJSON), &out); err != nil {
		return nil
	}
	return out
}

// Evaluation is the per-domain scoring output persisted for querying/exporting.
type Evaluation struct {
	ID                    uint   `gorm:"primaryKey"`
	Domain                string `gorm:"size:255;index"`
	DomainNormalized      string `gorm:"size:255;uniqueIndex"`
	// Trademark fields
	TrademarkScore          int     // Internal score (0-5) for AI use
	TrademarkType           string  `gorm:"size:32"`
	MatchedTrademark        string  `gorm:"size:255"`
	TrademarkConfidence     float64
	TrademarkRecommendation string  `gorm:"size:32;index"` // YES_RISK, POTENTIAL_RISK, NO_RISK
	// Vice fields
	ViceScore             int     // Internal score (0-5) for AI use
	ViceCategoriesJSON    string  `gorm:"type:text"`
	ViceConfidence        float64
	ViceRecommendation    string  `gorm:"size:32;index"` // YES_RISK, POTENTIAL_RISK, NO_RISK
	// Legacy field (kept for backwards compatibility, derived from TM + Vice)
	OverallRecommendation string `gorm:"size:32"`
	ProcessingTimeMs      int64
	Explanation           string `gorm:"type:text"`
	CommercialOverride    bool
	CommercialSource      string `gorm:"size:255"`
	CommercialSimilarity  float64
	// Manual override tracking
	ManualOverride   bool       `gorm:"default:false;index"`
	FeedbackUsedJSON string     `gorm:"type:text"`
	LastOverrideAt   *time.Time `gorm:"index"`
	OverrideCount    int        `gorm:"default:0"`
	CreatedAt        time.Time  `gorm:"autoCreateTime"`
}

// SetFeedbackUsed stores the IDs of feedback embeddings used during evaluation.
func (e *Evaluation) SetFeedbackUsed(ids []uint) {
	if ids == nil {
		e.FeedbackUsedJSON = "[]"
		return
	}
	payload, _ := json.Marshal(ids)
	e.FeedbackUsedJSON = string(payload)
}

// FeedbackUsed returns the IDs of feedback embeddings that were used.
func (e *Evaluation) FeedbackUsed() []uint {
	if strings.TrimSpace(e.FeedbackUsedJSON) == "" {
		return nil
	}
	var out []uint
	if err := json.Unmarshal([]byte(e.FeedbackUsedJSON), &out); err != nil {
		return nil
	}
	return out
}

// EvaluationOverride stores a manual override with full audit trail.
type EvaluationOverride struct {
	ID               uint   `gorm:"primaryKey"`
	EvaluationID     uint   `gorm:"index;not null"`
	DomainNormalized string `gorm:"size:255;index"`

	// Original values (snapshot at time of override)
	OriginalTrademarkScore          int
	OriginalTrademarkRecommendation string `gorm:"size:32"`
	OriginalViceScore               int
	OriginalViceRecommendation      string `gorm:"size:32"`
	OriginalRecommendation          string `gorm:"size:32"` // Legacy overall
	OriginalExplanation             string `gorm:"type:text"`

	// Override values
	OverrideTrademarkScore          *int   `gorm:"type:integer"`
	OverrideTrademarkRecommendation string `gorm:"size:32"` // YES_RISK, POTENTIAL_RISK, NO_RISK
	OverrideViceScore               *int   `gorm:"type:integer"`
	OverrideViceRecommendation      string `gorm:"size:32"` // YES_RISK, POTENTIAL_RISK, NO_RISK
	OverrideRecommendation          string `gorm:"size:32"` // Legacy overall (derived)
	OverrideExplanation             string `gorm:"type:text"`

	// Audit fields
	OverriddenBy string `gorm:"size:128;index;not null"`
	Reason       string `gorm:"type:text;not null"`

	// Feedback tracking
	FeedbackApplied bool  `gorm:"default:false"`
	EmbeddingID     *uint `gorm:"index"`

	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// FeedbackEmbedding stores vector embeddings for feedback retrieval.
type FeedbackEmbedding struct {
	ID               uint   `gorm:"primaryKey"`
	OverrideID       uint   `gorm:"uniqueIndex"`
	DomainNormalized string `gorm:"size:255;index"`
	SecondLevelLabel string `gorm:"size:255"`

	// The text that was embedded (for debugging/display)
	EmbeddingText string `gorm:"type:text;not null"`

	// Vector storage (JSON array of float64)
	// Using text-embedding-3-small: 1536 dimensions
	EmbeddingJSON      string `gorm:"type:text;not null"`
	EmbeddingDimension int    `gorm:"default:1536"`

	// Feedback metadata (denormalized for retrieval)
	CorrectedTrademarkRecommendation string `gorm:"size:32"`
	CorrectedViceRecommendation      string `gorm:"size:32"`
	CorrectedRecommendation          string `gorm:"size:32"` // Legacy overall
	CorrectedExplanation             string `gorm:"type:text"`
	CorrectedTrademarkScore          *int   `gorm:"type:integer"`
	CorrectedViceScore               *int   `gorm:"type:integer"`

	// Usage tracking
	RetrievalCount  int        `gorm:"default:0"`
	LastRetrievedAt *time.Time

	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// SetEmbedding stores the embedding vector as JSON.
func (f *FeedbackEmbedding) SetEmbedding(vector []float64) {
	if vector == nil {
		f.EmbeddingJSON = "[]"
		f.EmbeddingDimension = 0
		return
	}
	payload, _ := json.Marshal(vector)
	f.EmbeddingJSON = string(payload)
	f.EmbeddingDimension = len(vector)
}

// Embedding returns the parsed embedding vector.
func (f *FeedbackEmbedding) Embedding() []float64 {
	if strings.TrimSpace(f.EmbeddingJSON) == "" {
		return nil
	}
	var out []float64
	if err := json.Unmarshal([]byte(f.EmbeddingJSON), &out); err != nil {
		return nil
	}
	return out
}

// CSVBatch represents an uploaded CSV dataset.
type CSVBatch struct {
	ID               uint   `gorm:"primaryKey"`
	Name             string `gorm:"size:128;index"`
	Owner            string `gorm:"size:128;index"`
	OriginalFilename string `gorm:"size:256"`
	RowCount         int
	UniqueDomains    int
	ExistingDomains  int
	DuplicateRows    int
	ProcessedDomains int
	LastEvaluatedAt  *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// BatchRequest tracks an evaluation job for a batch (e.g., initial run, resume).
type BatchRequest struct {
	ID         uint   `gorm:"primaryKey"`
	BatchID    uint   `gorm:"index"`
	Type       string `gorm:"size:32"`
	Status     string `gorm:"size:32"`
	JobID      string `gorm:"size:64"`
	StartedAt  time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
}

// DomainBatch links domains to CSV batches (one row per domain occurrence).
type DomainBatch struct {
	ID               uint   `gorm:"primaryKey"`
	BatchID          uint   `gorm:"index"`
	Domain           string `gorm:"size:255;index"`
	DomainNormalized string `gorm:"size:255;index"`
	RowIndex         int
	CreatedAt        time.Time
}

// JobState persists evaluation job metadata across restarts.
type JobState struct {
	JobID         string `gorm:"primaryKey;size:64"`
	BatchID       uint   `gorm:"index"`
	RequestID     uint
	Status        string `gorm:"size:32;index"`
	Message       string `gorm:"size:255"`
	Processed     int
	Total         int64
	LastEventJSON string `gorm:"type:text"`
	UpdatedAt     time.Time
	CreatedAt     time.Time
}

// CommercialSale stores historical sales used to override vice/trademark risk decisions.
type CommercialSale struct {
	ID         uint   `gorm:"primaryKey"`
	SLD        string `gorm:"size:255"`
	Normalized string `gorm:"size:255;index"`
	Prefix     string `gorm:"size:16;index"`
	Length     int    `gorm:"index"`
	Price      float64
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}

// SetViceCategories saves the vice categories as JSON.
func (e *Evaluation) SetViceCategories(categories []string) {
	payload, _ := json.Marshal(categories)
	e.ViceCategoriesJSON = string(payload)
}

// ViceCategories returns the decoded vice categories slice.
func (e *Evaluation) ViceCategories() []string {
	if strings.TrimSpace(e.ViceCategoriesJSON) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(e.ViceCategoriesJSON), &out); err != nil {
		return nil
	}
	return out
}

// ScoreToRecommendation converts a numeric score (0-5) to a recommendation string.
// 4-5 → YES_RISK (high risk)
// 2-3 → POTENTIAL_RISK (needs review)
// 0-1 → NO_RISK (safe)
func ScoreToRecommendation(score int) string {
	if score >= 4 {
		return "YES_RISK"
	}
	if score >= 2 {
		return "POTENTIAL_RISK"
	}
	return "NO_RISK"
}

// DeriveRecommendations populates TrademarkRecommendation and ViceRecommendation from scores.
// Only sets values if not already set (AI decision takes priority).
func (e *Evaluation) DeriveRecommendations() {
	// Only derive from scores if not already set by AI
	if e.TrademarkRecommendation == "" {
		e.TrademarkRecommendation = ScoreToRecommendation(e.TrademarkScore)
	}
	if e.ViceRecommendation == "" {
		e.ViceRecommendation = ScoreToRecommendation(e.ViceScore)
	}
	// Only derive overall if not already set
	if e.OverallRecommendation == "" {
		if e.TrademarkRecommendation == "YES_RISK" || e.ViceRecommendation == "YES_RISK" {
			e.OverallRecommendation = "YES_RISK"
		} else if e.TrademarkRecommendation == "POTENTIAL_RISK" || e.ViceRecommendation == "POTENTIAL_RISK" {
			e.OverallRecommendation = "POTENTIAL_RISK"
		} else {
			e.OverallRecommendation = "NO_RISK"
		}
	}
}

// AITrainingTerm stores user-defined terms to teach the AI.
type AITrainingTerm struct {
	ID             uint      `gorm:"primaryKey"`
	Term           string    `gorm:"size:256;not null;uniqueIndex:idx_term_category"`
	Classification string    `gorm:"size:32;not null;index"` // YES_RISK or NO_RISK
	Category       string    `gorm:"size:32;not null;uniqueIndex:idx_term_category;default:trademark"` // trademark or vice
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}
