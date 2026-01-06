package scoring

import "strings"

// OverallResult merges trademark and vice outcomes into a recommendation.
type OverallResult struct {
	Recommendation string  `json:"overall_recommendation"`
	Confidence     float64 `json:"confidence"`
}

// CombineRecommendation provides a default recommendation.
// The AI will make the final smart decision based on context.
// This just provides a starting point.
func CombineRecommendation(tr TrademarkResult, vice ViceResult) OverallResult {
	// Default to POTENTIAL_RISK - let AI decide
	rec := "POTENTIAL_RISK"

	// High vice score is always a problem
	if vice.Score >= 4 {
		rec = "YES_RISK"
	}

	confidence := vice.Confidence
	if tr.Confidence < vice.Confidence {
		confidence = tr.Confidence
	}

	return OverallResult{
		Recommendation: strings.ToUpper(rec),
		Confidence:     confidence,
	}
}
