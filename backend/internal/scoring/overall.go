package scoring

import "strings"

// OverallResult merges trademark and vice outcomes into a recommendation.
type OverallResult struct {
	Recommendation string  `json:"overall_recommendation"`
	Confidence     float64 `json:"confidence"`
}

// CombineRecommendation applies BRD matrix logic to produce overall recommendation.
func CombineRecommendation(tr TrademarkResult, vice ViceResult) OverallResult {
	rec := "ALLOW"
	if tr.Score >= 4 || vice.Score >= 4 {
		rec = "BLOCK"
	} else if vice.Score == 3 || tr.Score == 3 {
		rec = "REVIEW"
	} else if tr.Score == 2 {
		rec = "ALLOW_WITH_CAUTION"
	} else if tr.Score == 1 {
		rec = "ALLOW_WITH_CAUTION"
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
