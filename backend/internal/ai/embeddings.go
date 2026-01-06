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

	"github.com/sirupsen/logrus"
)

// EmbeddingClient wraps the OpenAI embeddings API.
type EmbeddingClient struct {
	httpClient *http.Client
	apiKey     string
	model      string
	baseURL    string
}

// EmbeddingConfig holds configuration for the embedding client.
type EmbeddingConfig struct {
	APIKey  string
	Model   string
	BaseURL string
}

var (
	ErrEmbeddingDisabled = errors.New("embedding client disabled")
	ErrEmptyInput        = errors.New("empty input text")
)

// DefaultEmbeddingModel is the model used for generating embeddings.
const DefaultEmbeddingModel = "text-embedding-3-small"

// EmbeddingDimension is the output dimension for text-embedding-3-small.
const EmbeddingDimension = 1536

// NewEmbeddingClient constructs an EmbeddingClient if the configuration is valid.
func NewEmbeddingClient(cfg EmbeddingConfig) (*EmbeddingClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, ErrEmbeddingDisabled
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultEmbeddingModel
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &EmbeddingClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
	}, nil
}

// Enabled reports whether the client can make outbound calls.
func (c *EmbeddingClient) Enabled() bool {
	return c != nil && c.apiKey != ""
}

// GenerateEmbedding creates a vector embedding for the input text.
func (c *EmbeddingClient) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	if c == nil || !c.Enabled() {
		return nil, ErrEmbeddingDisabled
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmptyInput
	}

	payload := map[string]any{
		"model": c.model,
		"input": text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedding request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		var apiErr map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		logrus.WithFields(logrus.Fields{
			"status":   resp.StatusCode,
			"duration": duration,
			"error":    apiErr,
		}).Error("embedding API error")
		return nil, fmt.Errorf("openai embedding status %d: %v", resp.StatusCode, apiErr)
	}

	var decoded embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	if len(decoded.Data) == 0 {
		return nil, errors.New("openai embedding empty response")
	}

	embedding := decoded.Data[0].Embedding
	if len(embedding) == 0 {
		return nil, errors.New("openai embedding vector empty")
	}

	logrus.WithFields(logrus.Fields{
		"model":      c.model,
		"dimension":  len(embedding),
		"input_len":  len(text),
		"duration":   duration,
		"tokens_used": decoded.Usage.TotalTokens,
	}).Debug("generated embedding")

	return embedding, nil
}

// embeddingResponse represents the OpenAI embeddings API response.
type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns a value between -1 and 1, where 1 means identical direction.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// BatchGenerateEmbeddings generates embeddings for multiple texts in a single request.
// This is more efficient than calling GenerateEmbedding multiple times.
func (c *EmbeddingClient) BatchGenerateEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	if c == nil || !c.Enabled() {
		return nil, ErrEmbeddingDisabled
	}

	if len(texts) == 0 {
		return nil, ErrEmptyInput
	}

	// Filter and trim texts
	validTexts := make([]string, 0, len(texts))
	for _, text := range texts {
		trimmed := strings.TrimSpace(text)
		if trimmed != "" {
			validTexts = append(validTexts, trimmed)
		}
	}

	if len(validTexts) == 0 {
		return nil, ErrEmptyInput
	}

	payload := map[string]any{
		"model": c.model,
		"input": validTexts,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai batch embedding request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		var apiErr map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		logrus.WithFields(logrus.Fields{
			"status":   resp.StatusCode,
			"duration": duration,
			"error":    apiErr,
		}).Error("batch embedding API error")
		return nil, fmt.Errorf("openai batch embedding status %d: %v", resp.StatusCode, apiErr)
	}

	var decoded embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode batch embedding response: %w", err)
	}

	if len(decoded.Data) == 0 {
		return nil, errors.New("openai batch embedding empty response")
	}

	// Sort embeddings by index to maintain order
	embeddings := make([][]float64, len(validTexts))
	for _, item := range decoded.Data {
		if item.Index >= 0 && item.Index < len(embeddings) {
			embeddings[item.Index] = item.Embedding
		}
	}

	logrus.WithFields(logrus.Fields{
		"model":       c.model,
		"batch_size":  len(validTexts),
		"duration":    duration,
		"tokens_used": decoded.Usage.TotalTokens,
	}).Debug("generated batch embeddings")

	return embeddings, nil
}
