package scoring

import (
	"encoding/json"
	"os"
	"testing"

	"domain-risk-eval/backend/internal/match"
	"domain-risk-eval/backend/internal/store"
)

func TestTrademarkScoringExactMatchOnly(t *testing.T) {
	master := store.Mark{Serial: "3", Mark: "Master", MarkNoSpaces: "master"}
	marks := []store.Mark{
		{Serial: "1", Mark: "GOOGLE", MarkNoSpaces: "google", IsFanciful: true},
		{Serial: "2", Mark: "Amazon", MarkNoSpaces: "amazon"},
		master,
	}

	seedPath := createSeedFile(t, []string{"google"})
	scorer, err := NewTrademarkScorer(marks, seedPath)
	if err != nil {
		t.Fatalf("new scorer: %v", err)
	}

	testCases := []struct {
		name        string
		domain      string
		expectScore int
		expectType  string
		expectMatch string
	}{
		{"exact fanciful", "https://google.store", 5, "fanciful", "GOOGLE"},
		{"exact popular", "amazon.io", 5, "popular", "Amazon"},
		{"generic dictionary word", "master.ai", 2, "generic", "Master"},
		{"variant should ignore", "googl.store", 0, "none", ""},
		{"compound should ignore", "amazonmarket.shop", 0, "none", ""},
		{"ccTLD sld mismatch", "amazon.co.uk", 0, "none", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := match.NormalizeDomain(tc.domain)
			result := scorer.Score(profile)
			if result.Score != tc.expectScore {
				t.Fatalf("expected score %d got %d", tc.expectScore, result.Score)
			}
			if result.Type != tc.expectType {
				t.Fatalf("expected type %q got %q", tc.expectType, result.Type)
			}
			if result.MatchedTrademark != tc.expectMatch {
				t.Fatalf("expected matched trademark %q got %q", tc.expectMatch, result.MatchedTrademark)
			}
		})
	}
}

func createSeedFile(t *testing.T, seeds []string) string {
	t.Helper()
	tmp, err := os.CreateTemp(t.TempDir(), "seed-*.json")
	if err != nil {
		t.Fatalf("seed temp: %v", err)
	}
	if _, err := tmp.WriteString(toJSON(t, seeds)); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("seed close: %v", err)
	}
	return tmp.Name()
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}
