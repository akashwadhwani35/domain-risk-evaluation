package ai

// Decision captures the structured response expected from the AI explainer.
type Decision struct {
	// New structured fields
	WordType        string `json:"word_type,omitempty"`         // famous_brand, common_word, ambiguous, unknown
	WordMeaning     string `json:"word_meaning,omitempty"`      // What does this word mean?
	IsInventedName  *bool  `json:"is_invented_name,omitempty"`  // Is this a made-up brand name?
	FamousBrandMatch string `json:"famous_brand_match,omitempty"` // Which famous brand does it match?
	Decision        string `json:"decision,omitempty"`          // YES_RISK, NO_RISK, POTENTIAL_RISK
	Explanation     string `json:"explanation,omitempty"`       // Helpful explanation

	// Legacy fields for backwards compatibility
	Narrative      string   `json:"narrative,omitempty"`
	TrademarkScore *int     `json:"trademark_score,omitempty"`
	ViceScore      *int     `json:"vice_score,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
	Confidence     *float64 `json:"confidence,omitempty"`
	FamousMatch    *bool    `json:"famous_match,omitempty"`
	FamousLabel    string   `json:"famous_label,omitempty"`
}
