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
	TrademarkScore        int
	TrademarkType         string `gorm:"size:32"`
	MatchedTrademark      string `gorm:"size:255"`
	TrademarkConfidence   float64
	ViceScore             int
	ViceCategoriesJSON    string `gorm:"type:text"`
	ViceConfidence        float64
	OverallRecommendation string `gorm:"size:32"`
	ProcessingTimeMs      int64
	Explanation           string `gorm:"type:text"`
	CommercialOverride    bool
	CommercialSource      string `gorm:"size:255"`
	CommercialSimilarity  float64
	CreatedAt             time.Time `gorm:"autoCreateTime"`
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
