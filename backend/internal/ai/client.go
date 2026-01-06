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
		cfg.Model = "gpt-4.1-nano"
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
			"content": "You are a domain risk analyst. Reply with a strict JSON object containing keys narrative, trademark_score, vice_score, recommendation, confidence, famous_match, and famous_label. Evaluate trademark_score and vice_score as integers 0-5 (5 = severe conflict, 0 = clean) using the supplied evidence. Apply these rules:\n1. If the second-level label or any domain token is a well-known brand, personality, product, organization, or entertainment property (popular_token == true), set trademark_score to 5, recommendation to BLOCK, famous_match to true, and famous_label to that entity—even when heuristics suggested otherwise.\n2. If generic_popular == true (common English word heavily used by multiple parties such as crown, jeep, delta, liberty), cap trademark_score at 3 and recommendation at REVIEW unless additional evidence proves a famous-exclusive claim.\n3. If fanciful_unknown == true (invented spelling with no evidence of fame), default trademark_score to 3 and recommendation to REVIEW unless other evidence warrants escalation.\n4. Respect vice signals: vice_score >=4 must BLOCK, vice_score ==3 must at least REVIEW.\n5. Vice and trademark reasoning should combine logically; if both are clean, ALLOW or ALLOW_WITH_CAUTION only when appropriate.\nNarrative must contain exactly two sentences separated by a newline, the first sentence must reference the second-level label directly, and the second sentence must justify the action using concrete evidence. Do not start sentences with 'The term', 'Overall', 'I', 'I'd', 'Feels like', or 'It comes across', and avoid repeating opening clauses. Do not prefix the second sentence with labels like 'Stance:' or 'Recommendation:'. Vary vocabulary between responses. recommendation must be one of BLOCK, REVIEW, ALLOW_WITH_CAUTION, or ALLOW. confidence must be a decimal between 0 and 1. famous_match must be a boolean, famous_label must be concise or empty when famous_match is false. Emit nothing outside the JSON object, and never include the character \"\" inside any JSON string value.",
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
	fmt.Fprintf(builder, "Popular token: %t\n", input.PopularToken)
	fmt.Fprintf(builder, "Generic popular token: %t\n", input.GenericPopular)
	fmt.Fprintf(builder, "Fanciful unknown token: %t\n", input.FancifulUnknown)
	if len(input.DomainTokens) > 0 {
		fmt.Fprintf(builder, "Domain tokens: %s\n", strings.Join(input.DomainTokens, ", "))
	}
	fmt.Fprintf(builder, "Trademark Score: %d (%s)\n", input.Trademark.Score, input.Trademark.Type)
	if input.Trademark.MatchedTrademark != "" {
		fmt.Fprintf(builder, "Matched Trademark: %s\n", input.Trademark.MatchedTrademark)
	}
	fmt.Fprintf(builder, "Trademark Confidence: %.2f\n", input.Trademark.Confidence)
	fmt.Fprintf(builder, "Vice Score: %d\n", input.Vice.Score)
	if len(input.Vice.Categories) > 0 {
		fmt.Fprintf(builder, "Vice Categories: %s\n", strings.Join(input.Vice.Categories, ", "))
	}
	if len(input.ViceTerms) > 0 {
		fmt.Fprintf(builder, "Vice Terms: %s\n", strings.Join(input.ViceTerms, ", "))
	}
	fmt.Fprintf(builder, "Vice Confidence: %.2f\n", input.Vice.Confidence)
	fmt.Fprintf(builder, "Overall Recommendation: %s (confidence %.2f)\n", input.Overall.Recommendation, input.Overall.Confidence)
	if input.MarksCount > 0 {
		fmt.Fprintf(builder, "Marks in database: %d\n", input.MarksCount)
	}
	if input.DomainsCount > 0 {
		fmt.Fprintf(builder, "Domains evaluated in batch: %d\n", input.DomainsCount)
	}
	if len(input.CloseMatches) > 0 {
		fmt.Fprintf(builder, "Closest trademark references: %s\n", strings.Join(input.CloseMatches, "; "))
	}
	if input.Recommendation != "" {
		fmt.Fprintf(builder, "Default recommendation: %s\n", strings.ToUpper(input.Recommendation))
	}
	if input.AllowOverride {
		builder.WriteString("You may override the default recommendation if contextual evidence supports doing so.\n")
	}
	if input.HasSubstringAlerts {
		builder.WriteString("Some vice terms appear only as substrings of larger words; consider whether they are false positives.\n")
	}
	if strings.TrimSpace(input.CommercialSource) != "" && input.CommercialSimilarity > 0 {
		prefix := "Commercial context"
		if input.CommercialOverride {
			prefix = "Commercial signal"
		}
		builder.WriteString(fmt.Sprintf("%s: %s (similarity %.2f).\n", prefix, strings.TrimSpace(input.CommercialSource), input.CommercialSimilarity))
	} else if input.CommercialOverride && input.CommercialPrice > 0 {
		builder.WriteString(fmt.Sprintf("Commercial signal: historical sale around $%.0f supports market demand.\n", input.CommercialPrice))
	}
	builder.WriteString("Adjust scores based on evidence if needed.\n")
	if len(input.CloseMatches) > 0 {
		builder.WriteString("Exact matches indicate high-risk conflict.\n")
	} else {
		builder.WriteString("No exact USPTO matches found.\n")
	}
	if cue := strings.TrimSpace(input.OpeningCue); cue != "" {
		builder.WriteString(fmt.Sprintf("Begin the first sentence with \"%s\" while keeping the tone natural and directly referencing the second-level label.\n", cue))
	}
	// Include feedback from past corrections if available
	if feedback := strings.TrimSpace(input.FeedbackContext); feedback != "" {
		builder.WriteString("\n")
		builder.WriteString(feedback)
		builder.WriteString("\n")
	}
	second := strings.TrimSpace(input.SecondLevel)
	top := strings.TrimSpace(input.TopLevel)
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
	decision.Narrative = strings.TrimSpace(decision.Narrative)
	if decision.FamousLabel != "" {
		decision.FamousLabel = strings.TrimSpace(decision.FamousLabel)
	}
	if decision.TrademarkScore != nil {
		val := clampInt(*decision.TrademarkScore, 0, 5)
		decision.TrademarkScore = &val
	}
	if decision.ViceScore != nil {
		val := clampInt(*decision.ViceScore, 0, 5)
		decision.ViceScore = &val
	}
	decision.Recommendation = strings.ToUpper(strings.TrimSpace(decision.Recommendation))
	switch decision.Recommendation {
	case "BLOCK", "REVIEW", "ALLOW_WITH_CAUTION", "ALLOW":
	default:
		decision.Recommendation = ""
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
