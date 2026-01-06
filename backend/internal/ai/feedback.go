package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"domain-risk-eval/backend/internal/store"
)

// FeedbackRetriever retrieves similar past feedback for AI learning.
type FeedbackRetriever struct {
	embeddingClient *EmbeddingClient
	db              *store.Database
	topK            int
	minSimilarity   float64
}

// FeedbackRetrieverConfig holds configuration for the feedback retriever.
type FeedbackRetrieverConfig struct {
	TopK          int     // Number of similar feedback items to retrieve (default: 3)
	MinSimilarity float64 // Minimum cosine similarity threshold (default: 0.75)
}

// FeedbackRetrievalResult represents a single retrieved feedback item.
type FeedbackRetrievalResult struct {
	EmbeddingID             uint
	Domain                  string
	SecondLevelLabel        string
	Similarity              float64
	CorrectedRecommendation string
	CorrectedExplanation    string
	CorrectedTrademarkScore *int
	CorrectedViceScore      *int
}

// NewFeedbackRetriever creates a new FeedbackRetriever.
func NewFeedbackRetriever(embeddingClient *EmbeddingClient, db *store.Database, cfg FeedbackRetrieverConfig) *FeedbackRetriever {
	topK := cfg.TopK
	if topK <= 0 {
		topK = 3
	}
	minSim := cfg.MinSimilarity
	if minSim <= 0 {
		minSim = 0.75
	}

	return &FeedbackRetriever{
		embeddingClient: embeddingClient,
		db:              db,
		topK:            topK,
		minSimilarity:   minSim,
	}
}

// Enabled reports whether the retriever can perform searches.
func (r *FeedbackRetriever) Enabled() bool {
	return r != nil && r.embeddingClient != nil && r.embeddingClient.Enabled() && r.db != nil
}

// RetrieveSimilar finds feedback embeddings similar to the query domain context.
func (r *FeedbackRetriever) RetrieveSimilar(
	ctx context.Context,
	domain string,
	secondLevel string,
	trademarkScore int,
	viceScore int,
	recommendation string,
) ([]FeedbackRetrievalResult, error) {
	if !r.Enabled() {
		return nil, nil
	}

	// Build the query text from domain context
	queryText := BuildFeedbackText(domain, secondLevel, trademarkScore, viceScore, recommendation, "")

	// Generate embedding for the query
	start := time.Now()
	queryEmbedding, err := r.embeddingClient.GenerateEmbedding(ctx, queryText)
	if err != nil {
		logrus.WithError(err).WithField("domain", domain).Warn("failed to generate query embedding for feedback retrieval")
		return nil, err
	}

	// Get all stored feedback embeddings
	embeddings, err := r.db.GetAllEmbeddings()
	if err != nil {
		logrus.WithError(err).Warn("failed to retrieve feedback embeddings")
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, nil
	}

	// Calculate similarities and rank
	type scoredResult struct {
		embedding  store.FeedbackEmbedding
		similarity float64
	}

	scored := make([]scoredResult, 0, len(embeddings))
	for _, emb := range embeddings {
		vec := emb.Embedding()
		if len(vec) == 0 {
			continue
		}
		sim := CosineSimilarity(queryEmbedding, vec)
		if sim >= r.minSimilarity {
			scored = append(scored, scoredResult{
				embedding:  emb,
				similarity: sim,
			})
		}
	}

	// Sort by similarity descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].similarity > scored[j].similarity
	})

	// Take top K results
	limit := r.topK
	if limit > len(scored) {
		limit = len(scored)
	}

	results := make([]FeedbackRetrievalResult, limit)
	embeddingIDs := make([]uint, limit)

	for i := 0; i < limit; i++ {
		s := scored[i]
		results[i] = FeedbackRetrievalResult{
			EmbeddingID:             s.embedding.ID,
			Domain:                  s.embedding.DomainNormalized,
			SecondLevelLabel:        s.embedding.SecondLevelLabel,
			Similarity:              s.similarity,
			CorrectedRecommendation: s.embedding.CorrectedRecommendation,
			CorrectedExplanation:    s.embedding.CorrectedExplanation,
			CorrectedTrademarkScore: s.embedding.CorrectedTrademarkScore,
			CorrectedViceScore:      s.embedding.CorrectedViceScore,
		}
		embeddingIDs[i] = s.embedding.ID
	}

	// Update retrieval counts asynchronously
	if len(embeddingIDs) > 0 {
		go func() {
			if err := r.db.IncrementEmbeddingRetrievalCount(embeddingIDs); err != nil {
				logrus.WithError(err).Warn("failed to update embedding retrieval counts")
			}
		}()
	}

	logrus.WithFields(logrus.Fields{
		"domain":         domain,
		"total_searched": len(embeddings),
		"matches_found":  len(results),
		"top_similarity": func() float64 {
			if len(results) > 0 {
				return results[0].Similarity
			}
			return 0
		}(),
		"duration": time.Since(start),
	}).Debug("retrieved similar feedback")

	return results, nil
}

// FormatFeedbackForPrompt formats retrieved feedback results for inclusion in the AI prompt.
func FormatFeedbackForPrompt(results []FeedbackRetrievalResult) string {
	if len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("PAST FEEDBACK EXAMPLES (learn from these corrections):\n")

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("\nExample %d (similarity: %.2f):\n", i+1, r.Similarity))
		sb.WriteString(fmt.Sprintf("- Domain pattern: %s\n", r.SecondLevelLabel))
		sb.WriteString(fmt.Sprintf("- Corrected recommendation: %s\n", r.CorrectedRecommendation))
		if r.CorrectedTrademarkScore != nil {
			sb.WriteString(fmt.Sprintf("- Corrected trademark score: %d\n", *r.CorrectedTrademarkScore))
		}
		if r.CorrectedViceScore != nil {
			sb.WriteString(fmt.Sprintf("- Corrected vice score: %d\n", *r.CorrectedViceScore))
		}
		if r.CorrectedExplanation != "" {
			// Truncate long explanations
			explanation := r.CorrectedExplanation
			if len(explanation) > 200 {
				explanation = explanation[:197] + "..."
			}
			sb.WriteString(fmt.Sprintf("- Reasoning: %s\n", explanation))
		}
	}

	sb.WriteString("\nApply lessons from these examples when they are relevant to the current domain.\n")

	return sb.String()
}

// BuildFeedbackText constructs the text representation used for embedding.
// This captures the key characteristics of the domain evaluation.
func BuildFeedbackText(domain, secondLevel string, trademarkScore, viceScore int, recommendation, explanation string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Domain: %s\n", domain))
	sb.WriteString(fmt.Sprintf("Second-level label: %s\n", secondLevel))
	sb.WriteString(fmt.Sprintf("Trademark score: %d\n", trademarkScore))
	sb.WriteString(fmt.Sprintf("Vice score: %d\n", viceScore))
	sb.WriteString(fmt.Sprintf("Recommendation: %s\n", recommendation))

	if explanation != "" {
		// Include explanation but limit length to avoid token bloat
		exp := explanation
		if len(exp) > 500 {
			exp = exp[:497] + "..."
		}
		sb.WriteString(fmt.Sprintf("Explanation: %s\n", exp))
	}

	return sb.String()
}

// GetFeedbackIDs returns the embedding IDs from retrieval results.
func GetFeedbackIDs(results []FeedbackRetrievalResult) []uint {
	ids := make([]uint, len(results))
	for i, r := range results {
		ids[i] = r.EmbeddingID
	}
	return ids
}
