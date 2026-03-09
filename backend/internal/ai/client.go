package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"domain-risk-eval/backend/internal/scoring"
	"github.com/sirupsen/logrus"
)

// Explainer exposes AI-backed explanations for evaluation results.
type Explainer interface {
	Enabled() bool
	Explain(ctx context.Context, input ExplanationInput) (Decision, error)
}

// Config holds OpenAI configuration parameters.
type Config struct {
	APIKey      string
	Model       string
	BaseURL     string
	Temperature float64
	MaxTokens   int
}

// ExplanationInput describes the signals that feed the AI explanation.
type ExplanationInput struct {
	Domain               string
	Trademark            scoring.TrademarkResult
	Vice                 scoring.ViceResult
	Overall              scoring.OverallResult
	PopularToken         bool
	GenericPopular       bool
	FancifulUnknown      bool
	OpeningCue           string
	MarksCount           int
	DomainsCount         int
	CloseMatches         []string
	SecondLevel          string
	TopLevel             string
	DomainTokens         []string
	ViceTerms            []string
	Recommendation       string
	AllowOverride        bool
	HasSubstringAlerts   bool
	CommercialOverride   bool
	CommercialSource     string
	CommercialSimilarity float64
	CommercialPrice      float64
	FeedbackContext      string // Past feedback examples formatted for AI learning
}

// Client implements the Explainer interface against the OpenAI API.
type Client struct {
	httpClient  *http.Client
	apiKey      string
	model       string
	baseURL     string
	temperature float64
	maxTokens   int
}

var (
	ErrDisabled    = errors.New("ai explainer disabled")
	ErrInvalidJSON = errors.New("ai response invalid json")
)

type invalidJSONError struct {
	err error
}

func (e *invalidJSONError) Error() string {
	return fmt.Sprintf("parse ai response: %v", e.err)
}

func (e *invalidJSONError) Unwrap() error {
	return ErrInvalidJSON
}

// NewClient constructs a Client if the supplied configuration is valid.
func NewClient(cfg Config) (*Client, error) {
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Model == "" {
		cfg.Model = "gpt-4.1-mini"
	}
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrDisabled
	}
	temp := cfg.Temperature
	if temp <= 0 {
		temp = 0.2
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 300
	}
	client := &Client{
		httpClient:  &http.Client{Timeout: 60 * time.Second},
		apiKey:      strings.TrimSpace(cfg.APIKey),
		model:       cfg.Model,
		baseURL:     cfg.BaseURL,
		temperature: temp,
		maxTokens:   cfg.MaxTokens,
	}
	return client, nil
}

// Enabled reports whether the client can make outbound calls.
func (c *Client) Enabled() bool {
	return c != nil && c.apiKey != ""
}

// Explain requests an AI-generated explanation for a domain evaluation.
func (c *Client) Explain(ctx context.Context, input ExplanationInput) (Decision, error) {
	if c == nil || !c.Enabled() {
		return Decision{}, ErrDisabled
	}

	payload := c.buildPayload(input)
	body, err := json.Marshal(payload)
	if err != nil {
		return Decision{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Decision{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return Decision{}, fmt.Errorf("openai status %d: %v", resp.StatusCode, apiErr)
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Decision{}, fmt.Errorf("decode response: %w", err)
	}

	if len(decoded.Choices) == 0 {
		return Decision{}, errors.New("openai empty response")
	}

	content := normalizeJSONBlock(decoded.Choices[0].Message.Content)
	if content == "" {
		return Decision{}, errors.New("openai empty narrative")
	}

	content = sanitizeModelJSON(content)

	var decision Decision
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		logrus.WithError(err).WithField("content", content).Warn("ai response parse failed")
		return Decision{}, &invalidJSONError{err: err}
	}

	sanitizeDecision(&decision)
	if decision.Narrative == "" {
		return Decision{}, errors.New("ai narrative missing")
	}
	if decision.Recommendation == "" {
		return Decision{}, errors.New("ai recommendation missing")
	}

	return decision, nil
}

func normalizeJSONBlock(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		if idx := strings.IndexRune(trimmed, '\n'); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		if strings.HasSuffix(trimmed, "```") {
			trimmed = trimmed[:len(trimmed)-3]
		}
	}
	trimmed = strings.TrimSpace(trimmed)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		return strings.TrimSpace(trimmed[start : end+1])
	}
	return trimmed
}

var trailingNumberBeforeEOL = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\.[\s]*$`)

func sanitizeModelJSON(raw string) string {
	var sb strings.Builder
	sb.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch == '.' {
			prev := byte(0)
			if i > 0 {
				prev = raw[i-1]
			}
			next := byte(0)
			if i+1 < len(raw) {
				next = raw[i+1]
			}
			if prev >= '0' && prev <= '9' && (next < '0' || next > '9') {
				continue
			}
		}
		sb.WriteByte(ch)
	}
	cleaned := sb.String()
	cleaned = trailingNumberBeforeEOL.ReplaceAllString(cleaned, `$1`)
	cleaned = strings.ReplaceAll(cleaned, `..`, `.`)
	return cleaned
}

func (c *Client) buildPayload(input ExplanationInput) map[string]any {
	userPrompt := c.buildUserPrompt(input)
	messages := []map[string]string{
		{
			"role":    "system",
			"content": `You are a trademark lawyer assessing domain names. Use YOUR OWN knowledge of brands and trademarks. Do NOT rely on any system signals — decide independently.

Question: "Would a major company sue over this domain?"

YES_RISK - A company WILL sue (trademark):
- Exact famous brands: Google, Samsung, Microsoft, Nike, Disney, Netflix, Spotify, Pinterest, Uber, Tesla, Ferrari, Gucci, Rolex, Starbucks, McDonald's, Amazon, Apple, Facebook, Instagram, TikTok, Adidas, Toyota, Honda, BMW, Porsche, PayPal, Sony, Intel, Xerox, Lego
- Brand + generic word: samsungphone, googlesupport, nikeshop, teslaparts, amazonkindle, appleipad
- Famous products: iPod, iPhone, Xbox, Alexa, Gmail, Kindle, PlayStation
- Famous media: CNN, ESPN, HBO, BBC, MTV, TED/TEDx
- Minor misspellings/variations of above: amazn, googl, samsng, nkie

YES_RISK - Vice content (independent of trademark):
- Domains with obvious vulgar/sexual/offensive terms (porn, xxx, cock, pussy, fuck, cum, jizz, ass, dick, anal, nazi, rape, etc.)

NO_RISK - Nobody will sue (THIS IS THE DEFAULT):
- Random letters: aigo, omie, efit, visn, bcms, biox, xoft, mrkt, tvdb, mixr
- Dictionary words: david, already, enemy, join, ratio, diamond, voice, spirit, recovery, banking, home
- Generic terms: procurement, janitorial, locksmith, electrician, cybersex, threesome
- Person names: brian, amanda, david
- Abbreviations: rmbs, udoc, macg
- Foreign words: mutuo, offre, banche, acquistare
- If it's not a household brand name → NO_RISK

POTENTIAL_RISK - ONLY use when genuinely ambiguous:
- Famous brand that is ALSO a common word: Apple (fruit), Amazon (river), Shell (seashell)
- This should be RARE - less than 5% of domains

IMPORTANT:
- Assess trademark risk and vice risk SEPARATELY. A domain can have trademark risk without vice risk and vice versa.
- Your decision MUST match your explanation. If you say "no trademark risk" → decision MUST be NO_RISK.
- When in doubt about a brand, it's better to flag YES_RISK than to miss it.

OUTPUT JSON:
{
  "word_type": "famous_brand" | "common_word" | "random_letters" | "contains_brand",
  "famous_brand_match": "Brand name or empty",
  "trademark_score": 0-5,
  "vice_score": 0-5,
  "decision": "YES_RISK" | "NO_RISK" | "POTENTIAL_RISK",
  "explanation": "1 sentence"
}`,
		},
		{
			"role":    "user",
			"content": userPrompt,
		},
	}
	payload := map[string]any{
		"model":       c.model,
		"messages":    messages,
		"temperature": c.temperature,
	}
	if c.maxTokens > 0 {
		payload["max_tokens"] = c.maxTokens
	}
	return payload
}

func (c *Client) buildUserPrompt(input ExplanationInput) string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "Domain: %s\n", input.Domain)
	fmt.Fprintf(builder, "Second-level label: %s\n", strings.TrimSpace(input.SecondLevel))
	fmt.Fprintf(builder, "Top-level domain: %s\n", strings.TrimSpace(input.TopLevel))
	if len(input.DomainTokens) > 0 {
		fmt.Fprintf(builder, "Domain tokens: %s\n", strings.Join(input.DomainTokens, ", "))
	}
	// Only provide factual evidence — no heuristic scores or recommendations that anchor the AI.
	if input.Trademark.MatchedTrademark != "" {
		fmt.Fprintf(builder, "USPTO trademark match found: %s\n", input.Trademark.MatchedTrademark)
	}
	if len(input.Vice.Categories) > 0 {
		fmt.Fprintf(builder, "Vice terms detected: %s\n", strings.Join(input.Vice.Categories, ", "))
	} else if len(input.ViceTerms) > 0 {
		fmt.Fprintf(builder, "Vice terms detected: %s\n", strings.Join(input.ViceTerms, ", "))
	}
	if len(input.CloseMatches) > 0 {
		fmt.Fprintf(builder, "Close USPTO trademark matches: %s\n", strings.Join(input.CloseMatches, "; "))
	}
	if input.HasSubstringAlerts {
		builder.WriteString("Note: some flagged terms appear only as substrings of larger words; consider whether they are false positives.\n")
	}
	if strings.TrimSpace(input.CommercialSource) != "" && input.CommercialSimilarity > 0 {
		builder.WriteString(fmt.Sprintf("Commercial context: %s (similarity %.2f).\n", strings.TrimSpace(input.CommercialSource), input.CommercialSimilarity))
	}
	// Include feedback from past corrections if available
	if feedback := strings.TrimSpace(input.FeedbackContext); feedback != "" {
		builder.WriteString("\n")
		builder.WriteString(feedback)
		builder.WriteString("\n")
	}
	second := strings.TrimSpace(input.SecondLevel)
	top := strings.TrimSpace(input.TopLevel)
	if cue := strings.TrimSpace(input.OpeningCue); cue != "" {
		builder.WriteString(fmt.Sprintf("Begin the first sentence with \"%s\" while keeping the tone natural and directly referencing the second-level label.\n", cue))
	}
	if second != "" {
		builder.WriteString(fmt.Sprintf("Write two-sentence narrative: first describes \"%s\" intent with .%s context; second states action with justification. Vary openings, sound human, cite evidence.\n", second, top))
	} else {
		builder.WriteString("Write two-sentence narrative: first describes domain intent; second states action with justification. Vary openings, sound human, cite evidence.\n")
	}
	return builder.String()
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func sanitizeDecision(decision *Decision) {
	if decision == nil {
		return
	}

	// Handle new structured format
	decision.WordType = strings.ToLower(strings.TrimSpace(decision.WordType))
	decision.WordMeaning = strings.TrimSpace(decision.WordMeaning)
	decision.Explanation = strings.TrimSpace(decision.Explanation)
	decision.FamousBrandMatch = strings.TrimSpace(decision.FamousBrandMatch)

	// Normalize the decision field (new format uses "decision", old uses "recommendation")
	finalDecision := strings.ToUpper(strings.TrimSpace(decision.Decision))
	if finalDecision == "" {
		finalDecision = strings.ToUpper(strings.TrimSpace(decision.Recommendation))
	}

	// Map to valid 3-tier values
	switch finalDecision {
	case "YES_RISK", "NO_RISK", "POTENTIAL_RISK":
		// Already valid
	case "BLOCK":
		finalDecision = "YES_RISK"
	case "REVIEW", "ALLOW_WITH_CAUTION":
		finalDecision = "POTENTIAL_RISK"
	case "ALLOW":
		finalDecision = "NO_RISK"
	default:
		// Use word_type to infer decision if missing
		switch decision.WordType {
		case "famous_brand", "contains_brand":
			finalDecision = "YES_RISK"
		case "common_word", "generic_term":
			finalDecision = "NO_RISK"
		default:
			finalDecision = "NO_RISK" // Default to NO_RISK - be practical
		}
	}

	decision.Decision = finalDecision
	decision.Recommendation = finalDecision

	// Use explanation as narrative if narrative is empty
	if decision.Narrative == "" && decision.Explanation != "" {
		decision.Narrative = decision.Explanation
	}
	decision.Narrative = strings.TrimSpace(decision.Narrative)

	// Handle famous match from new format
	if decision.FamousBrandMatch != "" && decision.FamousLabel == "" {
		decision.FamousLabel = decision.FamousBrandMatch
	}
	if decision.FamousLabel != "" {
		decision.FamousLabel = strings.TrimSpace(decision.FamousLabel)
		if decision.FamousMatch == nil {
			match := true
			decision.FamousMatch = &match
		}
	}

	// Legacy field handling
	if decision.TrademarkScore != nil {
		val := clampInt(*decision.TrademarkScore, 0, 5)
		decision.TrademarkScore = &val
	}
	if decision.ViceScore != nil {
		val := clampInt(*decision.ViceScore, 0, 5)
		decision.ViceScore = &val
	}
	if decision.Confidence != nil {
		val := clampFloat(*decision.Confidence, 0, 1)
		decision.Confidence = &val
	}
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func clampFloat(value, min, max float64) float64 {
	if math.IsNaN(value) {
		return min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
