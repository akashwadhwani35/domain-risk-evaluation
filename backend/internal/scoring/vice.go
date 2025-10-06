package scoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"domain-risk-eval/backend/internal/match"
)

// ViceResult captures vice detection output.
type ViceResult struct {
	Score      int      `json:"score"`
	Categories []string `json:"categories"`
	Confidence float64  `json:"confidence"`
}

// ViceScorer evaluates domains against vice term lists.
type ViceScorer struct {
	terms map[int][]string
}

// NewViceScorer constructs a vice scorer from the provided JSON file.
func NewViceScorer(path string) (*ViceScorer, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read vice terms: %w", err)
	}
	var raw map[string][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal vice terms: %w", err)
	}
	terms := make(map[int][]string)
	for k, v := range raw {
		severity := atoiSafe(k)
		var list []string
		for _, term := range v {
			term = normalizeTerm(term)
			if term != "" {
				list = append(list, term)
			}
		}
		if len(list) > 0 {
			terms[severity] = list
		}
	}
	return &ViceScorer{terms: terms}, nil
}

// Score inspects the domain profile and returns vice scoring output.
func (v *ViceScorer) Score(profile match.DomainProfile) ViceResult {
	if v == nil {
		return ViceResult{Score: 0, Categories: nil, Confidence: 0.99}
	}

	domain := normalizeTerm(profile.Host)
	brand := normalizeTerm(profile.BrandToken)

	for severity := 5; severity >= 1; severity-- {
		terms := v.terms[severity]
		var hits []string
		for _, term := range terms {
			if term == "" {
				continue
			}
			if strings.Contains(domain, term) || strings.Contains(brand, term) {
				hits = append(hits, term)
			}
		}
		if len(hits) > 0 {
			return ViceResult{
				Score:      severity,
				Categories: dedupe(hits),
				Confidence: confidenceForSeverity(severity),
			}
		}
	}

	return ViceResult{Score: 0, Categories: nil, Confidence: confidenceForSeverity(0)}
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return in
	}
	sort.Strings(in)
	out := make([]string, 0, len(in))
	var prev string
	for _, item := range in {
		if item == prev {
			continue
		}
		out = append(out, item)
		prev = item
	}
	return out
}

func confidenceForSeverity(severity int) float64 {
	switch severity {
	case 5, 4:
		return 0.95
	case 3:
		return 0.80
	case 2:
		return 0.70
	case 1:
		return 0.60
	default:
		return 0.99
	}
}

func normalizeTerm(term string) string {
	term = strings.ToLower(term)
	term = strings.TrimSpace(term)
	return stripDiacritics(term)
}

func stripDiacritics(in string) string {
	var b strings.Builder
	b.Grow(len(in))
	for _, r := range in {
		if r == ' ' || r == '-' || r == '_' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
			continue
		}
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func atoiSafe(s string) int {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// Terms exposes the raw severity map (primarily for testing).
func (v *ViceScorer) Terms() map[int][]string {
	return v.terms
}

// Validate ensures the vice scorer has at least baseline configuration.
func (v *ViceScorer) Validate() error {
	if v == nil {
		return errors.New("vice scorer is nil")
	}
	if len(v.terms) == 0 {
		return errors.New("vice terms missing")
	}
	return nil
}
