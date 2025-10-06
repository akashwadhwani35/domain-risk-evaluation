package scoring

import (
	"encoding/json"
	"os"
	"testing"

	"domain-risk-eval/backend/internal/match"
)

func TestViceScoring(t *testing.T) {
	terms := map[string][]string{
		"5": {"terror"},
		"3": {"casino"},
		"1": {"dating"},
	}

	path := tempJSON(t, terms)
	scorer, err := NewViceScorer(path)
	if err != nil {
		t.Fatalf("vice scorer: %v", err)
	}

	tests := []struct {
		name     string
		domain   string
		expected int
	}{
		{"illegal", "terror-camp.com", 5},
		{"regulated", "play-casino.io", 3},
		{"low", "speed-dating.net", 1},
		{"clean", "flowers.store", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := match.NormalizeDomain(tc.domain)
			result := scorer.Score(profile)
			if result.Score != tc.expected {
				t.Fatalf("expected %d got %d", tc.expected, result.Score)
			}
		})
	}
}

func tempJSON(t *testing.T, value any) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "vice-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return f.Name()
}
