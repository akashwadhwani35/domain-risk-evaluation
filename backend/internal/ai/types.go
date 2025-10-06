package ai

// Decision captures the structured response expected from the AI explainer.
type Decision struct {
	Narrative      string   `json:"narrative"`
	TrademarkScore *int     `json:"trademark_score,omitempty"`
	ViceScore      *int     `json:"vice_score,omitempty"`
	Recommendation string   `json:"recommendation"`
	Confidence     *float64 `json:"confidence,omitempty"`
}
