package scoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"domain-risk-eval/backend/internal/match"
	"domain-risk-eval/backend/internal/store"
)

// TrademarkResult captures the outcome of a trademark evaluation.
type TrademarkResult struct {
	Score            int     `json:"score"`
	Type             string  `json:"type"`
	MatchedTrademark string  `json:"matched_trademark"`
	Confidence       float64 `json:"confidence"`
}

// TrademarkScorer evaluates domains against the trademark index.
type TrademarkScorer struct {
	index *trademarkIndex
}

// NewTrademarkScorer builds an index from the provided marks with seed overrides.
func NewTrademarkScorer(marks []store.Mark, seedPath string) (*TrademarkScorer, error) {
	seeds, err := loadSeeds(seedPath)
	if err != nil {
		return nil, err
	}
	idx := buildTrademarkIndex(marks, seeds)
	return &TrademarkScorer{index: idx}, nil
}

// Score computes the trademark risk score for the provided domain profile.
// Only fanciful exact matches between the domain's second-level label (SLD) and stored marks
// are considered a high-risk trademark hit. Popular brands or public figures trigger a medium
// review score, while generic words return a neutral score.
func (s *TrademarkScorer) Score(profile match.DomainProfile) TrademarkResult {
	if s == nil || s.index == nil {
		return TrademarkResult{Score: 0, Type: "none", Confidence: 0.2}
	}

	sld := sanitizeLabel(extractSLD(profile))
	if sld == "" {
		return TrademarkResult{Score: 0, Type: "none", Confidence: 0.2}
	}

	if entry := s.index.lookupExact(sld); entry != nil {
		markType := s.index.classify(entry)
		isCommon := isCommonWord(sld)
		switch markType {
		case "fanciful":
			if isCommon {
				return TrademarkResult{Score: 2, Type: "generic", MatchedTrademark: entry.Mark, Confidence: 0.6}
			}
			if IsPopularToken(sld) {
				return TrademarkResult{Score: 3, Type: "popular", MatchedTrademark: entry.Mark, Confidence: 0.9}
			}
			return TrademarkResult{Score: 5, Type: markType, MatchedTrademark: entry.Mark, Confidence: 1.0}
		case "popular":
			if isCommon {
				return TrademarkResult{Score: 2, Type: markType, MatchedTrademark: entry.Mark, Confidence: 0.75}
			}
			return TrademarkResult{Score: 3, Type: markType, MatchedTrademark: entry.Mark, Confidence: 0.9}
		default:
			if isCommon {
				return TrademarkResult{Score: 2, Type: "generic", MatchedTrademark: entry.Mark, Confidence: 0.6}
			}
			return TrademarkResult{Score: 0, Type: markType, MatchedTrademark: entry.Mark, Confidence: 0.4}
		}
	}

	return TrademarkResult{Score: 0, Type: "none", Confidence: 0.2}
}

// trademarkIndex stores precomputed mark lookups.
type trademarkIndex struct {
	exact map[string]*store.Mark
	seeds map[string]struct{}
}

func buildTrademarkIndex(marks []store.Mark, seeds map[string]struct{}) *trademarkIndex {
	return &trademarkIndex{
		exact: buildExactMap(marks),
		seeds: seeds,
	}
}

func (idx *trademarkIndex) lookupExact(token string) *store.Mark {
	if idx == nil {
		return nil
	}
	return idx.exact[token]
}

func (idx *trademarkIndex) classify(mark *store.Mark) string {
	if idx == nil || mark == nil {
		return "generic"
	}
	key := sanitizeLabel(mark.MarkNoSpaces)
	if _, ok := idx.seeds[key]; ok {
		return "fanciful"
	}
	if mark.IsFanciful {
		return "fanciful"
	}
	if IsPopularToken(key) {
		return "popular"
	}
	return "generic"
}

func buildExactMap(marks []store.Mark) map[string]*store.Mark {
	result := make(map[string]*store.Mark)
	for i := range marks {
		mark := &marks[i]
		key := sanitizeLabel(mark.MarkNoSpaces)
		if key == "" {
			continue
		}
		if _, exists := result[key]; exists {
			continue
		}
		result[key] = mark
	}
	return result
}

func loadSeeds(path string) (map[string]struct{}, error) {
	if path == "" {
		return map[string]struct{}{}, nil
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read seeds: %w", err)
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("unmarshal seeds: %w", err)
	}
	set := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		normalized := sanitizeLabel(entry)
		if normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set, nil
}

// LoadMarks loads marks from the database with an optional limit.
func LoadMarks(db *store.Database, limit int) ([]store.Mark, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	var marks []store.Mark
	start := time.Now()
	query := db.GORM().Table("popular_marks").
		Select("marks.*").
		Joins("JOIN marks ON marks.mark_no_spaces = popular_marks.normalized").
		Order("popular_marks.total DESC, marks.updated_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Scan(&marks).Error; err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"marks_limit": limit,
			"duration":    time.Since(start),
		}).Error("query popular marks failed")
		return nil, err
	}
	if len(marks) == 0 {
		logrus.WithFields(logrus.Fields{
			"marks_limit": limit,
			"duration":    time.Since(start),
		}).Warn("popular marks query returned no rows")
	}
	logrus.WithFields(logrus.Fields{
		"marks_returned": len(marks),
		"marks_limit":    limit,
		"duration":       time.Since(start),
	}).Info("queried popular marks for scoring")
	return marks, nil
}

func extractSLD(profile match.DomainProfile) string {
	host := strings.ToLower(strings.TrimSpace(profile.Host))
	if host == "" {
		return ""
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return parts[len(parts)-2]
}

func sanitizeLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
