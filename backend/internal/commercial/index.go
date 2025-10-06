package commercial

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"domain-risk-eval/backend/internal/store"
)

type Match struct {
	SLD        string
	Price      float64
	Similarity float64
}

// Service manages commercial sales persistence and lookup.
type Service struct {
	db      *store.Database
	cache   map[string]cacheEntry
	cacheMu sync.RWMutex
}

type cacheEntry struct {
	match Match
	found bool
}

func NewService(db *store.Database) *Service {
	return &Service{
		db:    db,
		cache: make(map[string]cacheEntry),
	}
}

// LoadFromCSV ingests the provided CSV and replaces the stored sales inventory.
func (s *Service) LoadFromCSV(path string, minPrice float64) (int, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, fmt.Errorf("commercial sales path is empty")
	}

	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open commercial sales file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReader(file))
	reader.FieldsPerRecord = -1

	var sales []store.CommercialSale
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read commercial sales row: %w", err)
		}
		if len(row) == 0 {
			continue
		}

		rawSLD := strings.TrimSpace(row[0])
		normalized := normalize(rawSLD)
		if normalized == "" || normalized == "sld" {
			continue
		}

		var price float64
		if len(row) > 1 {
			value := strings.TrimSpace(row[1])
			if value == "" || strings.EqualFold(value, "max_price") {
				continue
			}
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			price = parsed
		}

		if price < minPrice {
			continue
		}

		sale := store.CommercialSale{
			SLD:        rawSLD,
			Normalized: normalized,
			Prefix:     prefix(normalized, 3),
			Length:     runeLen(normalized),
			Price:      price,
		}
		sales = append(sales, sale)
	}

	if err := s.db.ReplaceCommercialSales(sales); err != nil {
		return 0, err
	}

	s.cacheMu.Lock()
	s.cache = make(map[string]cacheEntry)
	s.cacheMu.Unlock()

	return len(sales), nil
}

// Count returns the number of stored commercial sales rows.
func (s *Service) Count() int {
	if s == nil {
		return 0
	}
	count, err := s.db.CountCommercialSales()
	if err != nil {
		return 0
	}
	return int(count)
}

// BestMatch returns the best matching commercial sale for the supplied SLD.
func (s *Service) BestMatch(sld string) (Match, bool) {
	normalized := normalize(sld)
	if normalized == "" {
		return Match{}, false
	}

	if cachedMatch, ok := s.lookupCache(normalized); ok {
		return cachedMatch.match, cachedMatch.found
	}

	targetLen := runeLen(normalized)
	minLen := targetLen - 2
	if minLen < 1 {
		minLen = 1
	}
	maxLen := targetLen + 2

	prefix3 := prefix(normalized, 3)
	prefix2 := prefix(normalized, 2)
	prefix1 := prefix(normalized, 1)

	searchPrefixes := [][]string{
		uniqueNonEmpty([]string{prefix3}),
		uniqueNonEmpty([]string{prefix2}),
		uniqueNonEmpty([]string{prefix1}),
		nil,
	}

	var best Match
	var found bool

	for _, prefixes := range searchPrefixes {
		candidates, err := s.db.FindCommercialCandidates(prefixes, minLen, maxLen, targetLen, 75)
		if err != nil {
			continue
		}
		for _, candidate := range candidates {
			sim := similarity(normalized, candidate.Normalized)
			if sim > best.Similarity {
				best = Match{SLD: candidate.SLD, Price: candidate.Price, Similarity: sim}
				found = true
			}
		}
		if found && best.Similarity >= 0.95 {
			break
		}
	}

	s.storeCache(normalized, cacheEntry{match: best, found: found})
	if !found {
		return Match{}, false
	}
	return best, true
}

func (s *Service) lookupCache(key string) (cacheEntry, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	entry, ok := s.cache[key]
	return entry, ok
}

func (s *Service) storeCache(key string, entry cacheEntry) {
	s.cacheMu.Lock()
	s.cache[key] = entry
	s.cacheMu.Unlock()
}

func uniqueNonEmpty(items []string) []string {
	var result []string
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalize(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	return value
}

func prefix(value string, size int) string {
	if size <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) < size {
		size = len(runes)
	}
	if size <= 0 {
		return ""
	}
	return string(runes[:size])
}

func runeLen(value string) int {
	return len([]rune(value))
}

func similarity(a, b string) float64 {
	aRunes := []rune(a)
	bRunes := []rune(b)
	if len(aRunes) == 0 && len(bRunes) == 0 {
		return 1
	}
	if len(aRunes) == 0 || len(bRunes) == 0 {
		return 0
	}

	dist := levenshtein(aRunes, bRunes)
	maxLen := math.Max(float64(len(aRunes)), float64(len(bRunes)))
	if maxLen == 0 {
		return 1
	}

	score := 1 - float64(dist)/maxLen
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func levenshtein(a, b []rune) int {
	rows := len(a) + 1
	cols := len(b) + 1

	dp := make([]int, rows*cols)

	index := func(r, c int) int {
		return r*cols + c
	}

	for r := 0; r < rows; r++ {
		dp[index(r, 0)] = r
	}
	for c := 0; c < cols; c++ {
		dp[index(0, c)] = c
	}

	for r := 1; r < rows; r++ {
		for c := 1; c < cols; c++ {
			cost := 0
			if a[r-1] != b[c-1] {
				cost = 1
			}
			deletion := dp[index(r-1, c)] + 1
			insertion := dp[index(r, c-1)] + 1
			substitution := dp[index(r-1, c-1)] + cost
			dp[index(r, c)] = minInt(deletion, insertion, substitution)
		}
	}

	return dp[index(rows-1, cols-1)]
}

func minInt(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}
