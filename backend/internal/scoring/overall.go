package scoring

import "strings"

// OverallResult merges trademark and vice outcomes into a recommendation.
type OverallResult struct {
	Recommendation string  `json:"overall_recommendation"`
	Confidence     float64 `json:"confidence"`
}

// CombineRecommendation provides a default recommendation based on trademark and vice scores.
// NO_RISK is the default - most domains are safe.
// Only flag as risky when there's actual evidence.
func CombineRecommendation(tr TrademarkResult, vice ViceResult) OverallResult {
	// Default to NO_RISK - most domains are safe
	rec := "NO_RISK"
	confidence := 0.5

	// High vice score (4-5) = definite problem
	if vice.Score >= 4 {
		rec = "YES_RISK"
		confidence = vice.Confidence
	} else if vice.Score == 3 {
		// Moderate vice = potential risk
		rec = "POTENTIAL_RISK"
		confidence = vice.Confidence
	}

	// Trademark scoring:
	// Score 5 = fanciful/famous brand exact match → YES_RISK
	// Score 4 = fanciful variation (brand + suffix) → YES_RISK
	// Score 3 = popular brand → POTENTIAL_RISK
	// Score 2 = generic trademark match → NO_RISK (common words)
	// Score 1 = weak compound match → NO_RISK
	// Score 0 = no match → NO_RISK
	if tr.Score >= 4 {
		rec = "YES_RISK"
		confidence = tr.Confidence
	} else if tr.Score == 3 {
		// Popular brand - only upgrade if not already YES_RISK
		if rec != "YES_RISK" {
			rec = "POTENTIAL_RISK"
			confidence = tr.Confidence
		}
	}
	// Score 0-2: Keep as NO_RISK (no match, weak match, or generic dictionary word)

	return OverallResult{
		Recommendation: strings.ToUpper(rec),
		Confidence:     confidence,
	}
}
