package usp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Config drives USPTO client behaviour.
type Config struct {
	APIKey   string
	BaseURL  string
	Timeout  time.Duration
	CacheTTL time.Duration
	Rows     int
}

// Mark captures the subset of USPTO data we need for scoring.
type Mark struct {
	SerialNumber       string
	RegistrationNumber string
	Mark               string
	Owner              string
	Status             string
	StatusCode         string
	StatusCategory     string
	Classes            []string
	IsLive             bool
}

// LookupResult returns exact and similar matches for a query.
type LookupResult struct {
	Term         string
	ExactMatches []Mark
	Similar      []Mark
	Checked      bool
}

// Client performs USPTO API lookups with basic caching and rate limiting.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	rows       int
	cacheTTL   time.Duration
	cache      sync.Map // map[string]cacheEntry
}

type cacheEntry struct {
	at     time.Time
	result LookupResult
}

// ErrMissingCredentials is returned when the client cannot authenticate.
var ErrMissingCredentials = errors.New("usp client missing api key")

// NewClient constructs a USPTO client if configuration is valid.
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrMissingCredentials
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://developer.uspto.gov/ibd-api/v1/application/publications"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}

	rows := cfg.Rows
	if rows <= 0 {
		rows = 25
	}

	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		rows:       rows,
		cacheTTL:   ttl,
	}, nil
}

// LookupExact fetches USPTO data for the supplied trademark token.
func (c *Client) LookupExact(ctx context.Context, term string) (LookupResult, error) {
	if c == nil {
		return LookupResult{}, errors.New("usp client is nil")
	}

	key := strings.ToLower(strings.TrimSpace(term))
	if key == "" {
		return LookupResult{}, nil
	}

	if entry, ok := c.cache.Load(key); ok {
		cached := entry.(cacheEntry)
		if time.Since(cached.at) < c.cacheTTL {
			return cached.result, nil
		}
		c.cache.Delete(key)
	}

	result, err := c.performRequest(ctx, key)
	if err != nil {
		return LookupResult{}, err
	}

	c.cache.Store(key, cacheEntry{at: time.Now(), result: result})
	return result, nil
}

func (c *Client) performRequest(ctx context.Context, term string) (LookupResult, error) {
	params := url.Values{}
	params.Set("searchText", fmt.Sprintf("mark:(\"%s\") AND status:(\"LIVE\")", term))
	params.Set("rows", fmt.Sprintf("%d", c.rows))
	params.Set("start", "0")

	endpoint := c.baseURL
	if strings.Contains(endpoint, "?") {
		endpoint = endpoint + "&" + params.Encode()
	} else {
		endpoint = endpoint + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return LookupResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return LookupResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		// back off for 5 seconds and retry once
		select {
		case <-ctx.Done():
			return LookupResult{}, ctx.Err()
		case <-time.After(5 * time.Second):
		}
		// second attempt
		resp.Body.Close()
		retryReq, retryErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if retryErr != nil {
			return LookupResult{}, retryErr
		}
		retryReq.Header = req.Header.Clone()
		resp, err = c.httpClient.Do(retryReq)
		if err != nil {
			return LookupResult{}, err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return LookupResult{}, fmt.Errorf("usp to api status %d", resp.StatusCode)
	}

	var payload searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return LookupResult{}, fmt.Errorf("decode usp to response: %w", err)
	}

	cleanTerm := cleanKey(term)
	var exact []Mark
	var similar []Mark

	for _, item := range payload.Results {
		mark := strings.TrimSpace(item.MarkIdentification)
		if mark == "" {
			continue
		}
		record := Mark{
			SerialNumber:       strings.TrimSpace(item.SerialNumber),
			RegistrationNumber: strings.TrimSpace(item.RegistrationNumber),
			Mark:               mark,
			Owner:              strings.TrimSpace(item.OwnerName),
			Status:             strings.TrimSpace(item.MarkCurrentStatus),
			StatusCode:         strings.TrimSpace(item.MarkCurrentStatusCode),
			StatusCategory:     strings.TrimSpace(item.MarkCurrentStatusCategory),
			Classes:            collapseStrings(item.InternationalClasses),
		}
		statusUpper := strings.ToUpper(record.Status)
		categoryUpper := strings.ToUpper(record.StatusCategory)
		if strings.Contains(statusUpper, "LIVE") || strings.Contains(categoryUpper, "LIVE") {
			record.IsLive = true
		}

		if cleanKey(mark) == cleanTerm {
			exact = append(exact, record)
		} else {
			similar = append(similar, record)
		}
	}

	return LookupResult{
		Term:         term,
		ExactMatches: exact,
		Similar:      similar,
		Checked:      true,
	}, nil
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

type searchResult struct {
	SerialNumber              string      `json:"serialNumber"`
	RegistrationNumber        string      `json:"registrationNumber"`
	MarkIdentification        string      `json:"markIdentification"`
	MarkCurrentStatus         string      `json:"markCurrentStatus"`
	MarkCurrentStatusCode     string      `json:"markCurrentStatusCode"`
	MarkCurrentStatusCategory string      `json:"markCurrentStatusCategory"`
	OwnerName                 string      `json:"ownerName"`
	InternationalClasses      interface{} `json:"internationalClasses"`
}

func collapseStrings(raw interface{}) []string {
	switch v := raw.(type) {
	case []string:
		return dedupeStrings(v)
	case []interface{}:
		var out []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return dedupeStrings(out)
	case string:
		parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' })
		return dedupeStrings(parts)
	default:
		return nil
	}
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	var out []string
	for _, item := range items {
		key := strings.TrimSpace(strings.ToUpper(item))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, strings.TrimSpace(item))
	}
	return out
}

func cleanKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	return value
}
