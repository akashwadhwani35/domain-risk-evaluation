package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"domain-risk-eval/backend/internal/scoring"
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

var ErrDisabled = errors.New("ai explainer disabled")

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
		cfg.MaxTokens = 1500
	}
	client := &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
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

	var decision Decision
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return Decision{}, fmt.Errorf("parse ai response: %w", err)
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

func (c *Client) buildPayload(input ExplanationInput) map[string]any {
	userPrompt := c.buildUserPrompt(input)
	messages := []map[string]string{
		{
			"role":    "system",
			"content": "You are a domain risk analyst. Reply with a strict JSON object containing keys narrative, trademark_score, vice_score, recommendation, and confidence. Evaluate trademark_score and vice_score as integers 0-5 (5 = severe conflict, 0 = clean) using the supplied evidence; only assign 4-5 for clear exact-match conflicts or severe vice activity. Narrative must contain exactly two sentences separated by a newline, and the first sentence must reference the second-level label or its meaning directly. Do not start any sentence with 'The term', 'Overall', 'I', 'I'd', 'Feels like', or 'It comes across', and avoid repeating the same opening clause across responses. Do not prefix the second sentence with labels such as 'Stance:' or 'Recommendation:'; instead, lead with a varied action-oriented phrase that makes the decision sound human. Vary vocabulary and sentence structure between cases so successive narratives do not sound alike. recommendation must be one of BLOCK, REVIEW, ALLOW_WITH_CAUTION, or ALLOW. confidence must be a decimal between 0 and 1. Emit nothing outside the JSON object.",
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
	builder.WriteString("Heuristic trademark score suggestion (0-5): ")
	fmt.Fprintf(builder, "%d\n", input.Trademark.Score)
	builder.WriteString("Heuristic vice score suggestion (0-5): ")
	fmt.Fprintf(builder, "%d\n", input.Vice.Score)
	builder.WriteString("Use these heuristics as a starting point and adjust if the evidence supports a different outcome.\n")
	if len(input.CloseMatches) > 0 {
		builder.WriteString("Treat any listed mark that exactly matches the second-level label as a potential high-risk conflict.\n")
	} else {
		builder.WriteString("No exact USPTO matches were supplied; assume no direct conflict unless other evidence indicates otherwise.\n")
	}
	second := strings.TrimSpace(input.SecondLevel)
	top := strings.TrimSpace(input.TopLevel)
	if second != "" {
		builder.WriteString(fmt.Sprintf("Anchor the first sentence of the narrative in the meaning of the label \"%s\" and how the .%s TLD influences intent.\n", second, top))
	}
	builder.WriteString("Sound like a human analyst weighing intent, evidence, and risk cues—use fresh vocabulary each time.\n")
	builder.WriteString("Avoid repeating the exact domain string; instead, paraphrase the label's meaning in natural language.\n")
	builder.WriteString("Open the first sentence with a vivid description or plausible use case rather than a stock phrase.\n")
	builder.WriteString("Let the second sentence start with an action-oriented verb or directive (e.g., 'Greenlight', 'Flag', 'Escalate for legal eyes') while justifying the decision; never use the exact same starter twice.\n")
	builder.WriteString("Explain the likely use of the name, cite any trademark or vice evidence you spot, and mention commercial signals if they matter.\n")
	builder.WriteString("Populate the JSON fields with your final judgement. Narrative must include two sentences separated by a newline; vary how you introduce the recommendation in the second sentence while clearly stating the action and justification.\n")
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
