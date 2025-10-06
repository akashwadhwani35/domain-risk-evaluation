package scoring

import (
	"sync"

	"domain-risk-eval/backend/internal/store"
)

var (
	popularMu     sync.RWMutex
	popularTokens = defaultPopularTokens()
)

// SetPopularTokens replaces the in-memory popular token set. Existing defaults are merged to
// ensure we always keep a baseline of well-known brands even if the supplied map is empty.
func SetPopularTokens(tokens map[string]struct{}) {
	popularMu.Lock()
	defer popularMu.Unlock()

	combined := defaultPopularTokens()
	for token := range tokens {
		normalized := sanitizeLabel(token)
		if normalized == "" {
			continue
		}
		combined[normalized] = struct{}{}
	}
	popularTokens = combined
}

// IsPopularToken reports whether the supplied token is recognised as a popular brand or public
// figure that should trigger heightened review.
func IsPopularToken(token string) bool {
	normalized := sanitizeLabel(token)
	if normalized == "" {
		return false
	}
	popularMu.RLock()
	defer popularMu.RUnlock()
	_, ok := popularTokens[normalized]
	return ok
}

// LoadPopularTokensFromStore hydrates the in-memory set from the persisted popular mark table.
func LoadPopularTokensFromStore(db *store.Database, limit int) (int, error) {
	rows, err := db.ListPopularMarks(limit)
	if err != nil {
		return 0, err
	}
	set := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		normalized := sanitizeLabel(row.Normalized)
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	SetPopularTokens(set)
	return len(set), nil
}

// LoadPopularTokens aggregates popular marks on the fly and refreshes both the persisted table
// and the in-memory set.
func LoadPopularTokens(db *store.Database, limit, minCount int) (int, error) {
	popular, err := db.PopularMarks(limit, minCount)
	if err != nil {
		return 0, err
	}
	if err := db.ReplacePopularMarks(popular); err != nil {
		return 0, err
	}

	set := make(map[string]struct{}, len(popular))
	for _, row := range popular {
		normalized := sanitizeLabel(row.Normalized)
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	SetPopularTokens(set)
	return len(set), nil
}

func defaultPopularTokens() map[string]struct{} {
	return map[string]struct{}{
		"amazon":     {},
		"meta":       {},
		"facebook":   {},
		"google":     {},
		"youtube":    {},
		"instagram":  {},
		"twitter":    {},
		"tesla":      {},
		"microsoft":  {},
		"apple":      {},
		"netflix":    {},
		"paypal":     {},
		"uber":       {},
		"lyft":       {},
		"salesforce": {},
		"nike":       {},
		"adidas":     {},
		"cocacola":   {},
		"pepsi":      {},
		"tiktok":     {},
		"snapchat":   {},
		"beyonce":    {},
		"taylor":     {},
		"swift":      {},
		"kanye":      {},
		"elon":       {},
		"rihanna":    {},
		"drake":      {},
		"madonna":    {},
		"oprah":      {},
		"zuckerberg": {},
		"musk":       {},
	}
}
