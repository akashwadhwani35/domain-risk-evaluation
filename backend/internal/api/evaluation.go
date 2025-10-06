package api

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"domain-risk-eval/backend/internal/ai"
	"domain-risk-eval/backend/internal/match"
	"domain-risk-eval/backend/internal/scoring"
	"domain-risk-eval/backend/internal/store"
	"domain-risk-eval/backend/internal/usp"
	"domain-risk-eval/backend/internal/util"
)

const (
	evaluationThrottle            = 500 * time.Millisecond
	commercialSimilarityThreshold = 0.8
	aiMaxRetries                  = 3
	aiInitialBackoff              = 2 * time.Second
	aiMaxBackoff                  = 10 * time.Second
)

// evaluationJob tracks the state of a running evaluation.
type evaluationJob struct {
	id        string
	cancel    context.CancelFunc
	startedAt time.Time
	total     int64
	batchID   uint
	batchName string
	requestID uint
}

type domainResult struct {
	Evaluation     store.Evaluation
	LookupDuration time.Duration
	AiDuration     time.Duration
	TotalDuration  time.Duration
	Err            error
}

// startEvaluation launches a new asynchronous evaluation job. The caller must
// hold s.jobMu prior to invoking this function.
func (s *Server) startEvaluation(req EvaluateRequest, batch *store.CSVBatch, totalDomains int64) (*evaluationJob, error) {
	if s.activeJob != nil {
		return nil, errors.New("evaluation already running")
	}

	ctx, cancel := context.WithCancel(context.Background())
	job := &evaluationJob{
		id:        uuid.NewString(),
		cancel:    cancel,
		startedAt: time.Now().UTC(),
		total:     totalDomains,
		batchID:   batch.ID,
		batchName: batch.Name,
	}

	request, err := s.db.CreateBatchRequest(batch.ID, "evaluate", "running", job.id)
	if err != nil {
		job.cancel()
		return nil, fmt.Errorf("create batch request: %w", err)
	}
	job.requestID = request.ID

	s.activeJob = job
	go s.runEvaluation(ctx, job, req, batch)
	return job, nil
}

// cancelEvaluation aborts the active job if present.
func (s *Server) cancelEvaluation() {
	if s.activeJob == nil {
		return
	}
	s.activeJob.cancel()
}

func (s *Server) runEvaluation(ctx context.Context, job *evaluationJob, req EvaluateRequest, batch *store.CSVBatch) {
	finishStatus := "completed"
	var finishErr error

	defer func() {
		if job.requestID != 0 {
			status := finishStatus
			if finishErr != nil && status == "completed" {
				status = "failed"
			}
			if err := s.db.UpdateBatchRequest(job.requestID, status); err != nil {
				logrus.WithError(err).WithField("batch_id", job.batchID).Warn("update batch request")
			}
		}
		if err := s.db.UpdateBatchProcessingInfo(job.batchID); err != nil {
			logrus.WithError(err).WithField("batch_id", job.batchID).Warn("refresh batch processing info")
		}
		s.jobMu.Lock()
		s.activeJob = nil
		s.jobMu.Unlock()
	}()

	if req.Limit <= 0 {
		req.Limit = 5000
	}

	totalDomains := job.total
	if totalDomains <= 0 {
		finishStatus = "failed"
		s.evalNotifier.Broadcast(EvaluationEvent{
			Type:    "error",
			JobID:   job.id,
			BatchID: job.batchID,
			Message: "no domains available for evaluation",
		})
		return
	}

	marks, err := s.loadTrademarkMarks()
	if err != nil {
		finishStatus = "failed"
		finishErr = err
		s.evalNotifier.Broadcast(EvaluationEvent{
			Type:    "error",
			JobID:   job.id,
			BatchID: job.batchID,
			Message: fmt.Sprintf("load marks: %v", err),
		})
		logrus.WithError(err).Error("load marks")
		return
	}
	logrus.WithFields(logrus.Fields{
		"job":          job.id,
		"batch_id":     job.batchID,
		"marks_loaded": len(marks),
		"marks_limit":  s.marksLimit,
	}).Info("trademark marks ready for evaluation")

	trademarkScorer, err := scoring.NewTrademarkScorer(marks, s.seedPath)
	if err != nil {
		finishStatus = "failed"
		finishErr = err
		s.evalNotifier.Broadcast(EvaluationEvent{
			Type:    "error",
			JobID:   job.id,
			BatchID: job.batchID,
			Message: fmt.Sprintf("trademark scorer: %v", err),
		})
		logrus.WithError(err).Error("trademark scorer")
		return
	}

	skipExisting := req.Resume && !req.Force
	existing := make(map[string]struct{})
	totalProcessed := 0

	if skipExisting {
		evaluated, err := s.db.EvaluatedDomainsForBatch(job.batchID)
		if err != nil {
			finishStatus = "failed"
			finishErr = err
			s.evalNotifier.Broadcast(EvaluationEvent{
				Type:    "error",
				JobID:   job.id,
				BatchID: job.batchID,
				Message: fmt.Sprintf("load existing evaluations: %v", err),
			})
			logrus.WithError(err).Error("load existing evaluations")
			return
		}
		for _, dom := range evaluated {
			key := strings.TrimSpace(dom)
			if key != "" {
				existing[key] = struct{}{}
			}
		}
		totalProcessed = len(existing)
	}

	logrus.WithFields(logrus.Fields{
		"job":        job.id,
		"batch_id":   job.batchID,
		"batch_name": job.batchName,
		"total":      job.total,
		"processed":  totalProcessed,
		"resume":     req.Resume,
		"force":      req.Force,
	}).Info("evaluation job started")

	s.evalNotifier.Broadcast(EvaluationEvent{
		Type:      "started",
		JobID:     job.id,
		BatchID:   job.batchID,
		Total:     job.total,
		Processed: totalProcessed,
		Message:   "evaluation started",
	})

	workerCount := determineWorkerCount()
	logrus.WithFields(logrus.Fields{
		"job":      job.id,
		"batch_id": job.batchID,
		"workers":  workerCount,
	}).Info("evaluation worker pool configured")

	chunkSize := req.Limit
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if chunkSize > 5000 {
		chunkSize = 5000
	}

	taskCh := make(chan store.BatchDomain, workerCount*4)
	resultCh := make(chan domainResult, workerCount*4)
	errCh := make(chan error, 1)

	var (
		lastEmit     time.Time
		hasPending   bool
		pendingEvent EvaluationEvent
	)

	flush := func(force bool) {
		if !hasPending {
			return
		}
		if !force && !lastEmit.IsZero() && time.Since(lastEmit) < evaluationThrottle {
			return
		}
		ev := pendingEvent
		s.evalNotifier.Broadcast(ev)
		lastEmit = time.Now()
		logrus.WithFields(logrus.Fields{
			"job":       job.id,
			"batch_id":  job.batchID,
			"type":      ev.Type,
			"processed": ev.Processed,
			"total":     job.total,
		}).Debug("broadcast evaluation event")
		hasPending = false
	}

	var (
		usptoCacheMu sync.Mutex
		usptoCache   = make(map[string]usp.LookupResult)
	)

	var workerWG sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for task := range taskCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				res := s.evaluateDomain(ctx, task, trademarkScorer, marks, totalDomains, usptoCache, &usptoCacheMu)
				select {
				case resultCh <- res:
				case <-ctx.Done():
					return
				}
				if res.Err != nil {
					return
				}
			}
		}()
	}

	go func() {
		workerWG.Wait()
		close(resultCh)
	}()

	go func() {
		defer close(taskCh)
		defer close(errCh)
		offset := req.Offset
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			rows, err := s.db.ListBatchDomainsForEval(job.batchID, offset, chunkSize)
			if err != nil {
				errCh <- fmt.Errorf("list batch domains: %w", err)
				return
			}
			if len(rows) == 0 {
				return
			}
			for _, row := range rows {
				domainValue := strings.TrimSpace(row.Domain)
				if domainValue == "" {
					continue
				}
				normalizedKey := strings.TrimSpace(row.DomainNormalized)
				if normalizedKey == "" {
					normalizedKey = strings.ToLower(domainValue)
				}
				if skipExisting {
					if _, ok := existing[normalizedKey]; ok {
						continue
					}
				}
				taskCh <- store.BatchDomain{
					Domain:           domainValue,
					DomainNormalized: normalizedKey,
					RowIndex:         row.RowIndex,
				}
			}
			offset += len(rows)
			if len(rows) < chunkSize {
				return
			}
		}
	}()

	activeResultCh := resultCh
	activeErrCh := errCh
	done := false

	for activeResultCh != nil || activeErrCh != nil {
		select {
		case <-ctx.Done():
			flush(true)
			finishStatus = "cancelled"
			s.evalNotifier.Broadcast(EvaluationEvent{
				Type:      "cancelled",
				JobID:     job.id,
				BatchID:   job.batchID,
				Total:     job.total,
				Processed: totalProcessed,
				Message:   "evaluation cancelled",
			})
			logrus.WithField("job", job.id).WithField("batch_id", job.batchID).Warn("evaluation job cancelled via context")
			return
		case err, ok := <-activeErrCh:
			if !ok {
				activeErrCh = nil
				continue
			}
			if err != nil {
				flush(true)
				finishStatus = "failed"
				finishErr = err
				s.evalNotifier.Broadcast(EvaluationEvent{
					Type:    "error",
					JobID:   job.id,
					BatchID: job.batchID,
					Message: err.Error(),
				})
				logrus.WithError(err).Error("list batch domains")
				job.cancel()
				return
			}
		case res, ok := <-activeResultCh:
			if !ok {
				activeResultCh = nil
				continue
			}
			if done {
				continue
			}
			if res.Err != nil {
				flush(true)
				finishStatus = "failed"
				finishErr = res.Err
				s.evalNotifier.Broadcast(EvaluationEvent{
					Type:    "error",
					JobID:   job.id,
					BatchID: job.batchID,
					Message: fmt.Sprintf("evaluate domain: %v", res.Err),
				})
				logrus.WithError(res.Err).Error("evaluate domain")
				job.cancel()
				return
			}

			saveStart := time.Now()
			eval := res.Evaluation
			if err := s.db.SaveEvaluation(&eval); err != nil {
				flush(true)
				finishStatus = "failed"
				finishErr = err
				s.evalNotifier.Broadcast(EvaluationEvent{
					Type:    "error",
					JobID:   job.id,
					BatchID: job.batchID,
					Message: fmt.Sprintf("save evaluation: %v", err),
				})
				logrus.WithError(err).Error("save evaluation")
				job.cancel()
				return
			}
			saveDuration := time.Since(saveStart)

			if skipExisting {
				existing[eval.DomainNormalized] = struct{}{}
			}

			dto := FromModel(eval)
			totalProcessed++

			pendingEvent = EvaluationEvent{
				Type:       "evaluation",
				JobID:      job.id,
				BatchID:    job.batchID,
				Total:      job.total,
				Processed:  totalProcessed,
				Evaluation: &dto,
			}
			hasPending = true
			totalElapsed := res.TotalDuration + saveDuration
			logrus.WithFields(logrus.Fields{
				"job":           job.id,
				"batch_id":      job.batchID,
				"domain":        eval.Domain,
				"lookup_ms":     res.LookupDuration.Milliseconds(),
				"ai_ms":         res.AiDuration.Milliseconds(),
				"save_ms":       saveDuration.Milliseconds(),
				"processing_ms": eval.ProcessingTimeMs,
				"total_ms":      totalElapsed.Milliseconds(),
			}).Debug("evaluation timings")
			flush(false)

			if int64(totalProcessed) >= job.total {
				done = true
				job.cancel()
				continue
			}
		}
	}

	job.cancel()
	flush(true)

	duration := time.Since(job.startedAt).Round(time.Millisecond)
	s.evalNotifier.Broadcast(EvaluationEvent{
		Type:      "complete",
		JobID:     job.id,
		BatchID:   job.batchID,
		Total:     job.total,
		Processed: totalProcessed,
		Message:   fmt.Sprintf("evaluation finished in %s", duration),
	})
	logrus.WithFields(logrus.Fields{
		"job":       job.id,
		"batch_id":  job.batchID,
		"processed": totalProcessed,
		"duration":  duration,
	}).Info("evaluation job completed")
}

func determineWorkerCount() int {
	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	if workers > 12 {
		workers = 12
	}
	return workers
}

func (s *Server) evaluateDomain(
	ctx context.Context,
	domain store.BatchDomain,
	trademarkScorer *scoring.TrademarkScorer,
	marks []store.Mark,
	totalDomains int64,
	cache map[string]usp.LookupResult,
	cacheMu *sync.Mutex,
) domainResult {
	result := domainResult{}

	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}

	domainValue := strings.TrimSpace(domain.Domain)
	if domainValue == "" {
		result.Err = errors.New("empty domain value")
		return result
	}

	normalizedKey := strings.TrimSpace(domain.DomainNormalized)
	if normalizedKey == "" {
		normalizedKey = strings.ToLower(domainValue)
	}

	domainStart := time.Now()
	timer := util.StartTimer()
	profile := match.NormalizeDomain(domainValue)

	fallbackResult := trademarkScorer.Score(profile)

	lookupDuration := time.Duration(0)
	var lookupResult usp.LookupResult
	var lookupValid bool
	if s.usptoClient != nil {
		lookupStart := time.Now()
		if cacheMu != nil {
			cacheMu.Lock()
			lookupResult, lookupValid = s.lookupUSPTO(ctx, profile.BrandToken, cache)
			cacheMu.Unlock()
		} else {
			lookupResult, lookupValid = s.lookupUSPTO(ctx, profile.BrandToken, cache)
		}
		lookupDuration = time.Since(lookupStart)
	}

	trademarkResult, closeMatches := s.resolveTrademark(profile, lookupValid, lookupResult, fallbackResult)
	viceResult := s.viceScorer.Score(profile)
	overall := scoring.CombineRecommendation(trademarkResult, viceResult)

	commercialOverride := false
	commercialSource := ""
	commercialSimilarity := 0.0
	commercialPrice := 0.0

	secondLevel, topLevel := splitDomainParts(domainValue)
	if s.commercial != nil {
		if match, ok := s.commercial.BestMatch(secondLevel); ok && match.Similarity >= commercialSimilarityThreshold {
			commercialSimilarity = match.Similarity
			commercialPrice = match.Price
			commercialSource = fmt.Sprintf("sale $%.0f", match.Price)
			if viceResult.Score <= 2 && trademarkResult.Score <= 3 {
				commercialOverride = true
				switch overall.Recommendation {
				case "BLOCK":
					overall.Recommendation = "REVIEW"
				case "REVIEW":
					overall.Recommendation = "ALLOW_WITH_CAUTION"
				}
			}
		}
	}

	aiStart := time.Now()
	decision, err := s.generateDecision(
		ctx,
		profile,
		domainValue,
		marks,
		totalDomains,
		closeMatches,
		trademarkResult,
		viceResult,
		overall,
		secondLevel,
		topLevel,
		commercialOverride,
		commercialSource,
		commercialSimilarity,
		commercialPrice,
	)
	aiDuration := time.Since(aiStart)
	if err != nil {
		result.Err = err
		return result
	}

	if decision.TrademarkScore != nil {
		trademarkResult.Score = clampScore(*decision.TrademarkScore)
	}
	if decision.ViceScore != nil {
		viceResult.Score = clampScore(*decision.ViceScore)
	}

	overall = scoring.CombineRecommendation(trademarkResult, viceResult)
	if commercialOverride {
		switch overall.Recommendation {
		case "BLOCK":
			overall.Recommendation = "REVIEW"
		case "REVIEW":
			overall.Recommendation = "ALLOW_WITH_CAUTION"
		}
	}

	if rec := strings.ToUpper(strings.TrimSpace(decision.Recommendation)); rec != "" {
		overall.Recommendation = rec
	}
	if decision.Confidence != nil {
		conf := clampConfidence(*decision.Confidence)
		overall.Confidence = conf
		trademarkResult.Confidence = conf
		viceResult.Confidence = conf
	}

	eval := store.Evaluation{
		Domain:                domainValue,
		DomainNormalized:      normalizedKey,
		TrademarkScore:        trademarkResult.Score,
		TrademarkType:         trademarkResult.Type,
		MatchedTrademark:      trademarkResult.MatchedTrademark,
		TrademarkConfidence:   trademarkResult.Confidence,
		ViceScore:             viceResult.Score,
		ViceConfidence:        viceResult.Confidence,
		OverallRecommendation: overall.Recommendation,
		ProcessingTimeMs:      timer.ElapsedMs(),
		Explanation:           strings.TrimSpace(decision.Narrative),
		CommercialOverride:    commercialOverride,
		CommercialSource:      commercialSource,
		CommercialSimilarity:  commercialSimilarity,
	}
	eval.SetViceCategories(viceResult.Categories)

	result.Evaluation = eval
	result.LookupDuration = lookupDuration
	result.AiDuration = aiDuration
	result.TotalDuration = time.Since(domainStart)
	return result
}

func (s *Server) generateDecision(
	ctx context.Context,
	profile match.DomainProfile,
	domain string,
	marks []store.Mark,
	totalDomains int64,
	closeMatches []string,
	trademarkResult scoring.TrademarkResult,
	viceResult scoring.ViceResult,
	overall scoring.OverallResult,
	secondLevel string,
	topLevel string,
	commercialOverride bool,
	commercialSource string,
	commercialSimilarity float64,
	commercialPrice float64,
) (ai.Decision, error) {
	decision := ai.Decision{Recommendation: strings.ToUpper(strings.TrimSpace(overall.Recommendation))}
	if s.explainer == nil || !s.explainer.Enabled() {
		decision.Narrative = buildFallbackNarrative(overall.Recommendation)
		return decision, nil
	}

	tokens := collectDomainTokens(profile)
	viceTerms := append([]string{}, viceResult.Categories...)

	input := ai.ExplanationInput{
		Domain:               domain,
		Trademark:            trademarkResult,
		Vice:                 viceResult,
		Overall:              overall,
		MarksCount:           len(marks),
		DomainsCount:         int(totalDomains),
		CloseMatches:         closeMatches,
		SecondLevel:          secondLevel,
		TopLevel:             topLevel,
		DomainTokens:         tokens,
		ViceTerms:            viceTerms,
		Recommendation:       decision.Recommendation,
		AllowOverride:        true,
		HasSubstringAlerts:   hasSubstringAlerts(viceTerms, profile.Core, tokens),
		CommercialOverride:   commercialOverride,
		CommercialSource:     commercialSource,
		CommercialSimilarity: commercialSimilarity,
		CommercialPrice:      commercialPrice,
	}

	result, err := s.callAIWithRetry(ctx, input)
	if err != nil {
		logrus.WithError(err).Warn("ai explainer unavailable; falling back to heuristic output")
		decision.Narrative = buildFallbackNarrative(overall.Recommendation)
		return decision, nil
	}

	if strings.TrimSpace(result.Narrative) != "" {
		decision.Narrative = strings.TrimSpace(result.Narrative)
	}
	if rec := strings.ToUpper(strings.TrimSpace(result.Recommendation)); rec != "" {
		decision.Recommendation = rec
	}
	decision.TrademarkScore = result.TrademarkScore
	decision.ViceScore = result.ViceScore
	decision.Confidence = result.Confidence

	return decision, nil
}

func (s *Server) callAIWithRetry(ctx context.Context, input ai.ExplanationInput) (ai.Decision, error) {
	if s.explainer == nil || !s.explainer.Enabled() {
		return ai.Decision{}, ai.ErrDisabled
	}

	delay := aiInitialBackoff
	var lastErr error
	for attempt := 0; attempt < aiMaxRetries; attempt++ {
		decision, err := s.explainer.Explain(ctx, input)
		if err == nil {
			return decision, nil
		}

		lastErr = err
		if ctx.Err() != nil {
			return ai.Decision{}, ctx.Err()
		}

		if !shouldRetryAI(err) {
			break
		}

		select {
		case <-ctx.Done():
			return ai.Decision{}, ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > aiMaxBackoff {
			delay = aiMaxBackoff
		}
	}

	return ai.Decision{}, lastErr
}

func shouldRetryAI(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 429") || strings.Contains(msg, "status 500") || strings.Contains(msg, "status 503")
}

func buildFallbackNarrative(recommendation string) string {
	rec := strings.ToUpper(strings.TrimSpace(recommendation))
	switch rec {
	case "BLOCK":
		return "Heuristic risk scoring recommends BLOCK; AI explanation is temporarily unavailable."
	case "REVIEW":
		return "Heuristic signals recommend REVIEW; AI narrative waiting for retry."
	case "ALLOW_WITH_CAUTION":
		return "Heuristic evaluation suggests ALLOW WITH CAUTION; AI summary could not be retrieved."
	default:
		return "Heuristic evaluation completed; AI explanation unavailable at this time."
	}
}

func splitDomainParts(domain string) (string, string) {
	host := strings.ToLower(strings.TrimSpace(domain))
	parts := strings.Split(host, ".")
	if len(parts) == 0 {
		return host, ""
	}
	tld := parts[len(parts)-1]
	if len(parts) == 1 {
		return parts[0], tld
	}
	secondLevel := parts[len(parts)-2]
	return secondLevel, tld
}

func collectDomainTokens(profile match.DomainProfile) []string {
	seen := make(map[string]struct{})
	appendToken := func(value string) {
		trimmed := strings.TrimSpace(strings.ToLower(value))
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
	}

	appendToken(profile.BrandToken)
	appendToken(profile.Core)
	for _, token := range profile.Tokens {
		appendToken(token)
	}
	for _, token := range profile.AltSplits {
		appendToken(token)
	}

	result := make([]string, 0, len(seen))
	for token := range seen {
		result = append(result, token)
	}
	sortStrings(result)
	return result
}

func hasSubstringAlerts(terms []string, core string, tokens []string) bool {
	if len(terms) == 0 {
		return false
	}
	lowerCore := strings.ToLower(strings.TrimSpace(core))
	tokenSet := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		tokenSet[token] = struct{}{}
	}
	for _, term := range terms {
		termLower := strings.ToLower(strings.TrimSpace(term))
		if termLower == "" {
			continue
		}
		if _, ok := tokenSet[termLower]; ok {
			continue
		}
		if strings.Contains(lowerCore, termLower) {
			return true
		}
	}
	return false
}

func extractRecommendation(text string, fallback string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if !(strings.HasPrefix(lower, "recommended stance:") || strings.HasPrefix(lower, "stance:")) {
			continue
		}
		remainder := line
		if strings.HasPrefix(lower, "recommended stance:") {
			remainder = strings.TrimSpace(line[len("Recommended stance:"):])
		} else {
			remainder = strings.TrimSpace(line[len("Stance:"):])
		}
		remainder = strings.TrimLeft(remainder, "-–— ")
		upto := strings.Fields(remainder)
		if len(upto) == 0 {
			break
		}
		candidate := strings.ToUpper(strings.TrimSpace(upto[0]))
		switch candidate {
		case "BLOCK", "REVIEW", "ALLOW_WITH_CAUTION", "ALLOW":
			return candidate
		case "ALLOWWITHCAUTION":
			return "ALLOW_WITH_CAUTION"
		}
	}
	return strings.ToUpper(strings.TrimSpace(fallback))
}

func sortStrings(items []string) {
	if len(items) <= 1 {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
}

func clampScore(value int) int {
	if value < 0 {
		return 0
	}
	if value > 5 {
		return 5
	}
	return value
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
