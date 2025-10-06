package scoring

import "testing"

func TestCombineRecommendation(t *testing.T) {
	tests := []struct {
		name     string
		tr       TrademarkResult
		vice     ViceResult
		expected string
	}{
		{"tr block", TrademarkResult{Score: 5, Confidence: 1}, ViceResult{Score: 0, Confidence: 0.9}, "BLOCK"},
		{"vice block", TrademarkResult{Score: 0, Confidence: 0.9}, ViceResult{Score: 4, Confidence: 0.95}, "BLOCK"},
		{"review", TrademarkResult{Score: 3, Confidence: 0.7}, ViceResult{Score: 0, Confidence: 0.9}, "REVIEW"},
		{"allow with caution", TrademarkResult{Score: 1, Confidence: 0.5}, ViceResult{Score: 0, Confidence: 0.9}, "ALLOW_WITH_CAUTION"},
		{"allow", TrademarkResult{Score: 0, Confidence: 0.9}, ViceResult{Score: 0, Confidence: 0.99}, "ALLOW"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := CombineRecommendation(tc.tr, tc.vice)
			if result.Recommendation != tc.expected {
				t.Fatalf("expected %s got %s", tc.expected, result.Recommendation)
			}
		})
	}
}
