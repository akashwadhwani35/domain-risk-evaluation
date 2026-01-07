package store

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Database wraps the GORM DB handle and exposes repository helpers.
type Database struct {
	gorm *gorm.DB
	mu   sync.Mutex
}

// Open initializes the SQLite-backed database at the provided path.
func Open(path string, silent bool) (*Database, error) {
	cfg := &gorm.Config{}
	if silent {
		cfg.Logger = logger.Default.LogMode(logger.Silent)
	}
	db, err := gorm.Open(sqlite.Open(path), cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.AutoMigrate(&Mark{}, &Domain{}, &Evaluation{}, &CommercialSale{}, &PopularMark{}, &CSVBatch{}, &BatchRequest{}, &DomainBatch{}, &JobState{}, &EvaluationOverride{}, &FeedbackEmbedding{}, &AITrainingTerm{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		logrus.WithError(err).Warn("enable WAL mode")
	}
	if err := db.Exec("PRAGMA synchronous=NORMAL").Error; err != nil {
		logrus.WithError(err).Warn("set synchronous pragma")
	}
	// Wait up to 60 seconds for locks to clear instead of immediately failing
	if err := db.Exec("PRAGMA busy_timeout=60000").Error; err != nil {
		logrus.WithError(err).Warn("set busy_timeout pragma")
	}
	if err := applyIndexes(db); err != nil {
		return nil, fmt.Errorf("apply indexes: %w", err)
	}
	return &Database{gorm: db}, nil
}

// GORM exposes the raw gorm.DB handle.
func (d *Database) GORM() *gorm.DB {
	return d.gorm
}

// WithTransaction executes fn inside a database transaction.
func (d *Database) WithTransaction(fn func(*Database) error) error {
	if d == nil || d.gorm == nil {
		return errors.New("database is nil")
	}
	return d.gorm.Transaction(func(tx *gorm.DB) error {
		return fn(&Database{gorm: tx})
	})
}

// Close closes the underlying database connection.
func (d *Database) Close() error {
	if d == nil {
		return nil
	}
	sqlDB, err := d.gorm.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// UpsertMark inserts or updates an existing mark.
func (d *Database) UpsertMark(mark *Mark) error {
	if mark == nil {
		return errors.New("mark is nil")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gorm.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "serial"}},
		DoUpdates: clause.AssignmentColumns([]string{"registration", "mark", "mark_normalized", "mark_no_spaces", "owner", "classes_json", "is_fanciful", "updated_at"}),
	}).Create(mark).Error
}

// SaveDomain inserts or updates the domain record.
func (d *Database) SaveDomain(domain *Domain) error {
	if domain == nil {
		return errors.New("domain is nil")
	}
	domain.Domain = strings.TrimSpace(domain.Domain)
	domain.DomainNormalized = normalizeDomainKey(domain.Domain)
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gorm.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain_normalized"}},
		DoUpdates: clause.AssignmentColumns([]string{"domain", "brand_token", "tokens_json", "updated_at"}),
	}).Create(domain).Error
}

// SaveEvaluation creates an evaluation row.
func (d *Database) SaveEvaluation(e *Evaluation) error {
	if e == nil {
		return errors.New("evaluation is nil")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	// Derive recommendations from scores before saving
	e.DeriveRecommendations()
	columns := []string{
		"trademark_score",
		"trademark_type",
		"matched_trademark",
		"trademark_confidence",
		"trademark_recommendation",
		"vice_score",
		"vice_categories_json",
		"vice_confidence",
		"vice_recommendation",
		"overall_recommendation",
		"processing_time_ms",
		"explanation",
		"commercial_override",
		"commercial_source",
		"commercial_similarity",
	}
	e.Domain = strings.TrimSpace(e.Domain)
	e.DomainNormalized = normalizeDomainKey(e.Domain)
	return d.gorm.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "domain_normalized"}},
		DoUpdates: clause.AssignmentColumns(append(columns, "domain", "domain_normalized")),
	}).Create(e).Error
}

// EvaluatedDomains returns all domains that already have an evaluation row.
func (d *Database) EvaluatedDomains() ([]string, error) {
	if d == nil {
		return nil, errors.New("database is nil")
	}
	var domains []string
	if err := d.gorm.Model(&Evaluation{}).Pluck("domain", &domains).Error; err != nil {
		return nil, err
	}
	return domains, nil
}

// ReplaceCommercialSales swaps the existing sales inventory with the provided slice.
func (d *Database) ReplaceCommercialSales(sales []CommercialSale) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gorm.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&CommercialSale{}).Error; err != nil {
			return err
		}
		if len(sales) == 0 {
			return nil
		}
		const batchSize = 250
		for start := 0; start < len(sales); start += batchSize {
			end := start + batchSize
			if end > len(sales) {
				end = len(sales)
			}
			batch := sales[start:end]
			if err := tx.CreateInBatches(batch, batchSize).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CountCommercialSales returns the number of stored commercial sales entries.
func (d *Database) CountCommercialSales() (int64, error) {
	var count int64
	if err := d.gorm.Model(&CommercialSale{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// FindCommercialCandidates returns candidate sales rows filtered by optional prefixes and length bounds.
func (d *Database) FindCommercialCandidates(prefixes []string, minLen, maxLen, targetLen, limit int) ([]CommercialSale, error) {
	query := d.gorm.Model(&CommercialSale{})
	if minLen > 0 {
		query = query.Where("length >= ?", minLen)
	}
	if maxLen > 0 {
		query = query.Where("length <= ?", maxLen)
	}
	if len(prefixes) > 0 {
		query = query.Where("prefix IN ?", prefixes)
	}
	if targetLen > 0 {
		query = query.Order(clause.Expr{SQL: "ABS(length - ?)", Vars: []any{targetLen}})
	}
	query = query.Order("price DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []CommercialSale
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ClearEvaluations removes previously calculated evaluations (useful before re-processing).
func (d *Database) ClearEvaluations() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gorm.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Evaluation{}).Error
}

// ClearDomains removes existing domain entries (used before re-importing a CSV).
func (d *Database) ClearDomains() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gorm.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Domain{}).Error
}

// CountMarks returns the number of mark entries.
func (d *Database) CountMarks() (int64, error) {
	var count int64
	if err := d.gorm.Model(&Mark{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountDomains returns the domain count.
func (d *Database) CountDomains() (int64, error) {
	var count int64
	if err := d.gorm.Model(&Domain{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListDomains returns a paged set of domains ordered by ID.
func (d *Database) ListDomains(offset, limit int) ([]Domain, int64, error) {
	var domains []Domain
	var total int64
	if err := d.gorm.Model(&Domain{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := d.gorm.Model(&Domain{}).Order("id ASC")
	if limit > 0 {
		q = q.Offset(offset).Limit(limit)
	}
	if err := q.Find(&domains).Error; err != nil {
		return nil, 0, err
	}
	return domains, total, nil
}

// EvaluationQuery encapsulates filters and pagination for listing evaluation rows.
type EvaluationQuery struct {
	Query          string
	MinTrademark   int
	MinVice        int
	TLD            string
	Recommendation string
	Sort           string
	Offset         int
	Limit          int
	BatchID        uint
}

// ListEvaluations returns paginated evaluation records applying optional filters.
func (d *Database) ListEvaluations(opts EvaluationQuery) ([]Evaluation, int64, error) {
	var total int64
	base := d.gorm.Model(&Evaluation{})
	if opts.BatchID > 0 {
		base = base.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", opts.BatchID)
	}
	if opts.Query != "" {
		like := fmt.Sprintf("%%%s%%", opts.Query)
		base = base.Where("domain LIKE ? OR matched_trademark LIKE ?", like, like)
	}
	if opts.MinTrademark > 0 {
		base = base.Where("trademark_score >= ?", opts.MinTrademark)
	}
	if opts.MinVice > 0 {
		base = base.Where("vice_score >= ?", opts.MinVice)
	}
	if tld := strings.TrimSpace(opts.TLD); tld != "" {
		if !strings.HasPrefix(tld, ".") {
			tld = "." + tld
		}
		like := fmt.Sprintf("%%%s", strings.ToLower(tld))
		base = base.Where("LOWER(domain) LIKE ?", like)
	}
	if rec := strings.TrimSpace(opts.Recommendation); rec != "" {
		base = base.Where("overall_recommendation = ?", strings.ToUpper(rec))
	}

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := orderForSort(opts.Sort)
	queryBuilder := base.Order(order).Offset(opts.Offset)
	if opts.Limit > 0 {
		queryBuilder = queryBuilder.Limit(opts.Limit)
	}

	var rows []Evaluation
	if err := queryBuilder.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func orderForSort(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "domain_asc":
		return "evaluations.domain ASC"
	case "domain_desc":
		return "evaluations.domain DESC"
	case "trademark_desc":
		return "evaluations.trademark_score DESC, evaluations.vice_score DESC, evaluations.id DESC"
	case "trademark_asc":
		return "evaluations.trademark_score ASC, evaluations.id DESC"
	case "vice_desc":
		return "evaluations.vice_score DESC, evaluations.trademark_score DESC, evaluations.id DESC"
	case "vice_asc":
		return "evaluations.vice_score ASC, evaluations.id DESC"
	case "created_asc":
		return "evaluations.created_at ASC"
	case "created_desc":
		return "evaluations.created_at DESC"
	default:
		return "evaluations.id DESC"
	}
}

type BatchDomain struct {
	Domain           string
	DomainNormalized string
	RowIndex         int
	HasResult        bool
}

func normalizeDomainKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func applyIndexes(db *gorm.DB) error {
	stmts := []string{
		"UPDATE domains SET domain_normalized = LOWER(domain) WHERE domain IS NOT NULL AND (domain_normalized IS NULL OR domain_normalized = '')",
		"UPDATE domain_batches SET domain_normalized = LOWER(domain) WHERE domain IS NOT NULL AND (domain_normalized IS NULL OR domain_normalized = '')",
		"UPDATE evaluations SET domain_normalized = LOWER(domain) WHERE domain IS NOT NULL AND (domain_normalized IS NULL OR domain_normalized = '')",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_domains_domain_normalized ON domains(domain_normalized)",
		"CREATE INDEX IF NOT EXISTS idx_domains_brand_token ON domains(brand_token)",
		"CREATE INDEX IF NOT EXISTS idx_domain_batches_batch_id ON domain_batches(batch_id)",
		"CREATE INDEX IF NOT EXISTS idx_domain_batches_batch_domain_normalized ON domain_batches(batch_id, domain_normalized)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_evaluations_domain_normalized ON evaluations(domain_normalized)",
		"CREATE INDEX IF NOT EXISTS idx_evaluations_trademark_score ON evaluations(trademark_score)",
		"CREATE INDEX IF NOT EXISTS idx_evaluations_vice_score ON evaluations(vice_score)",
		"CREATE INDEX IF NOT EXISTS idx_marks_mark_normalized ON marks(mark_normalized)",
		"CREATE INDEX IF NOT EXISTS idx_marks_mark_no_spaces ON marks(mark_no_spaces)",
		"CREATE INDEX IF NOT EXISTS idx_marks_owner ON marks(owner)",
		"CREATE INDEX IF NOT EXISTS idx_popular_marks_total ON popular_marks(total DESC)",
		"CREATE INDEX IF NOT EXISTS idx_job_states_status_updated ON job_states(status, updated_at)",
		"CREATE INDEX IF NOT EXISTS idx_job_states_batch ON job_states(batch_id)",
		"CREATE INDEX IF NOT EXISTS idx_evaluation_overrides_evaluation_id ON evaluation_overrides(evaluation_id)",
		"CREATE INDEX IF NOT EXISTS idx_evaluation_overrides_domain ON evaluation_overrides(domain_normalized)",
		"CREATE INDEX IF NOT EXISTS idx_evaluation_overrides_user ON evaluation_overrides(overridden_by)",
		"CREATE INDEX IF NOT EXISTS idx_feedback_embeddings_domain ON feedback_embeddings(domain_normalized)",
		"CREATE INDEX IF NOT EXISTS idx_evaluations_manual_override ON evaluations(manual_override)",
		"CREATE INDEX IF NOT EXISTS idx_evaluations_last_override ON evaluations(last_override_at)",
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

// CreateCSVBatch inserts a new CSV batch record.
func (d *Database) CreateCSVBatch(name, owner, filename string) (*CSVBatch, error) {
	batch := &CSVBatch{Name: name, Owner: owner, OriginalFilename: filename}
	if err := d.gorm.Create(batch).Error; err != nil {
		return nil, err
	}
	return batch, nil
}

// UpdateCSVBatchStats updates aggregate statistics for a batch.
func (d *Database) UpdateCSVBatchStats(batchID uint, rowCount, uniqueDomains, existingDomains, duplicateRows, processed int) error {
	return d.gorm.Model(&CSVBatch{}).
		Where("id = ?", batchID).
		Updates(map[string]any{
			"row_count":         rowCount,
			"unique_domains":    uniqueDomains,
			"existing_domains":  existingDomains,
			"duplicate_rows":    duplicateRows,
			"processed_domains": processed,
		}).Error
}

// ReplaceDomainBatch replaces all domain entries associated with a batch.
func (d *Database) ReplaceDomainBatch(batchID uint, rows []DomainBatch) error {
	return d.gorm.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("batch_id = ?", batchID).Delete(&DomainBatch{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(rows, 500).Error
	})
}

// ExistingEvaluationKeys returns a set of domains that already have evaluation results.
func (d *Database) ExistingEvaluationKeys(domains []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if len(domains) == 0 {
		return result, nil
	}

	unique := make([]string, 0, len(domains))
	seen := make(map[string]struct{})
	for _, dom := range domains {
		key := normalizeDomainKey(dom)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}

	if len(unique) == 0 {
		return result, nil
	}

	const chunkSize = 1000
	for i := 0; i < len(unique); i += chunkSize {
		end := i + chunkSize
		if end > len(unique) {
			end = len(unique)
		}
		chunk := unique[i:end]

		var rows []string
		if err := d.gorm.Model(&Evaluation{}).
			Where("domain_normalized IN ?", chunk).
			Pluck("domain_normalized", &rows).Error; err != nil {
			return nil, err
		}
		for _, dom := range rows {
			result[dom] = struct{}{}
		}
	}
	return result, nil
}

// CountBatchDomains returns the number of distinct domains in a batch.
func (d *Database) CountBatchDomains(batchID uint) (int, error) {
	var count int64
	if err := d.gorm.Model(&DomainBatch{}).
		Where("batch_id = ?", batchID).
		Distinct("domain_normalized").Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

// CountBatchResults returns the number of domains in a batch that already have evaluation results.
func (d *Database) CountBatchResults(batchID uint) (int, error) {
	var count int64
	query := d.gorm.Table("domain_batches AS db").
		Select("COUNT(DISTINCT e.domain_normalized)").
		Joins("JOIN evaluations e ON e.domain_normalized = db.domain_normalized").
		Where("db.batch_id = ?", batchID)
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

// ListBatchDomainsForEval returns unique domains for a batch along with evaluation status.
func (d *Database) ListBatchDomainsForEval(batchID uint, offset, limit int) ([]BatchDomain, error) {
	var rows []BatchDomain
	query := `
		SELECT MIN(db.domain) AS domain,
		       db.domain_normalized AS domain_normalized,
		       MIN(db.row_index) AS row_index,
		       CASE WHEN SUM(CASE WHEN e.id IS NULL THEN 0 ELSE 1 END) > 0 THEN 1 ELSE 0 END AS has_result
		FROM domain_batches db
		LEFT JOIN evaluations e ON e.domain_normalized = db.domain_normalized
		WHERE db.batch_id = ?
		GROUP BY db.domain_normalized
		ORDER BY MIN(db.row_index)
		LIMIT ? OFFSET ?`
	if err := d.gorm.Raw(query, batchID, limit, offset).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// EvaluatedDomainsForBatch returns the normalized domains already evaluated for the batch.
func (d *Database) EvaluatedDomainsForBatch(batchID uint) ([]string, error) {
	var rows []string
	query := `
		SELECT DISTINCT e.domain_normalized
		FROM evaluations e
		JOIN domain_batches db ON db.domain_normalized = e.domain_normalized
		WHERE db.batch_id = ?`
	if err := d.gorm.Raw(query, batchID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CreateBatchRequest records a new evaluation request for a batch.
func (d *Database) CreateBatchRequest(batchID uint, requestType, status, jobID string) (*BatchRequest, error) {
	request := &BatchRequest{
		BatchID:   batchID,
		Type:      requestType,
		Status:    status,
		JobID:     jobID,
		StartedAt: time.Now(),
	}
	if err := d.gorm.Create(request).Error; err != nil {
		return nil, err
	}
	return request, nil
}

// UpdateBatchRequest updates the status and timestamps of a batch request.
func (d *Database) UpdateBatchRequest(requestID uint, status string) error {
	updates := map[string]any{"status": status}
	if status == "completed" || status == "failed" {
		now := time.Now()
		updates["finished_at"] = &now
	}
	return d.gorm.Model(&BatchRequest{}).Where("id = ?", requestID).Updates(updates).Error
}

// LatestRunningBatchRequest returns the most recent running batch request.
func (d *Database) LatestRunningBatchRequest() (*BatchRequest, error) {
	var request BatchRequest
	if err := d.gorm.
		Where("status = ?", "running").
		Order("started_at DESC").
		First(&request).Error; err != nil {
		return nil, err
	}
	return &request, nil
}

// UpsertJobState inserts or updates a job state record.
func (d *Database) UpsertJobState(state *JobState) error {
	if state == nil {
		return errors.New("job state is nil")
	}
	state.UpdatedAt = time.Now()
	return d.gorm.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"batch_id", "request_id", "status", "message", "processed", "total", "last_event_json", "updated_at"}),
	}).Create(state).Error
}

// GetLatestJobState returns the most recently updated job state.
func (d *Database) GetLatestJobState() (*JobState, error) {
	var state JobState
	if err := d.gorm.Order("updated_at DESC").First(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

// GetLatestRunningJobState returns the most recent running job state.
func (d *Database) GetLatestRunningJobState() (*JobState, error) {
	var state JobState
	if err := d.gorm.
		Where("status IN ?", []string{"started", "progress", "evaluation"}).
		Order("updated_at DESC").
		First(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

// DeleteEvaluationsForBatchSafe deletes evaluations for domains unique to the batch.
func (d *Database) DeleteEvaluationsForBatchSafe(batchID uint) (int64, error) {
	result := d.gorm.Exec(`
		DELETE FROM evaluations
		WHERE domain_normalized IN (
			SELECT domain_normalized FROM domain_batches WHERE batch_id = ?
		) AND domain_normalized NOT IN (
			SELECT domain_normalized FROM domain_batches WHERE batch_id != ?
		)`, batchID, batchID)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// DeleteDomainBatches removes all domain_batch rows for a batch.
func (d *Database) DeleteDomainBatches(batchID uint) error {
	return d.gorm.Where("batch_id = ?", batchID).Delete(&DomainBatch{}).Error
}

// DeleteBatchRequests removes batch requests for a batch.
func (d *Database) DeleteBatchRequests(batchID uint) error {
	return d.gorm.Where("batch_id = ?", batchID).Delete(&BatchRequest{}).Error
}

// DeleteJobStates removes job state records for a batch.
func (d *Database) DeleteJobStates(batchID uint) error {
	return d.gorm.Where("batch_id = ?", batchID).Delete(&JobState{}).Error
}

// DeleteCSVBatch removes the batch record itself.
func (d *Database) DeleteCSVBatch(batchID uint) error {
	return d.gorm.Where("id = ?", batchID).Delete(&CSVBatch{}).Error
}

// UpdateBatchProcessingInfo refreshes processed counts and timestamp for a batch.
func (d *Database) UpdateBatchProcessingInfo(batchID uint) error {
	processed, err := d.CountBatchResults(batchID)
	if err != nil {
		return err
	}
	now := time.Now()
	return d.gorm.Model(&CSVBatch{}).
		Where("id = ?", batchID).
		Updates(map[string]any{
			"processed_domains": processed,
			"last_evaluated_at": &now,
		}).Error
}

// ListCSVBatches returns CSV batches ordered by creation time.
func (d *Database) ListCSVBatches(offset, limit int) ([]CSVBatch, int64, error) {
	var total int64
	if err := d.gorm.Model(&CSVBatch{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	query := d.gorm.Model(&CSVBatch{}).Order("created_at DESC")
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	var batches []CSVBatch
	if err := query.Find(&batches).Error; err != nil {
		return nil, 0, err
	}
	return batches, total, nil
}

// GetCSVBatch retrieves a batch by ID.
func (d *Database) GetCSVBatch(batchID uint) (*CSVBatch, error) {
	var batch CSVBatch
	if err := d.gorm.First(&batch, batchID).Error; err != nil {
		return nil, err
	}
	return &batch, nil
}

// GetBatchRequest fetches a batch request record by ID.
func (d *Database) GetBatchRequest(requestID uint) (*BatchRequest, error) {
	var request BatchRequest
	if err := d.gorm.First(&request, requestID).Error; err != nil {
		return nil, err
	}
	return &request, nil
}

// RecommendationCount holds count per recommendation type.
type RecommendationCount struct {
	Recommendation string
	Count          int64
}

// ScoreBucket holds a score and its count.
type ScoreBucket struct {
	Score int
	Count int64
}

// BatchSummary holds aggregated statistics for a batch.
type BatchSummary struct {
	ID               uint       `json:"id"`
	Name             string     `json:"name"`
	Owner            string     `json:"owner"`
	TotalDomains     int        `json:"total_domains"`
	ProcessedDomains int        `json:"processed_domains"`
	// Trademark recommendation counts
	TrademarkYesRisk       int64 `json:"trademark_yes_risk"`
	TrademarkPotentialRisk int64 `json:"trademark_potential_risk"`
	TrademarkNoRisk        int64 `json:"trademark_no_risk"`
	// Vice recommendation counts
	ViceYesRisk       int64 `json:"vice_yes_risk"`
	VicePotentialRisk int64 `json:"vice_potential_risk"`
	ViceNoRisk        int64 `json:"vice_no_risk"`
	// Legacy overall counts (kept for backwards compatibility)
	YesRiskCount   int64      `json:"yes_risk_count"`
	PotentialCount int64      `json:"potential_count"`
	NoRiskCount    int64      `json:"no_risk_count"`
	CreatedAt      time.Time  `json:"created_at"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at"`
}

// TimeSeriesPoint represents a date and value for time-series data.
type TimeSeriesPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// GetRecommendationCounts returns counts grouped by overall_recommendation.
// If batchID > 0, counts are scoped to that batch.
// Maps both old (BLOCK/REVIEW/ALLOW) and new (YES_RISK/POTENTIAL_RISK/NO_RISK) values.
func (d *Database) GetRecommendationCounts(batchID uint) (map[string]int64, error) {
	result := map[string]int64{
		"YES_RISK":       0,
		"POTENTIAL_RISK": 0,
		"NO_RISK":        0,
	}

	query := d.gorm.Model(&Evaluation{}).
		Select("overall_recommendation as recommendation, COUNT(*) as count").
		Group("overall_recommendation")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	var rows []RecommendationCount
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		if row.Recommendation == "" {
			continue
		}
		// Map old values to new 3-tier system
		switch row.Recommendation {
		case "YES_RISK", "BLOCK":
			result["YES_RISK"] += row.Count
		case "POTENTIAL_RISK", "REVIEW", "ALLOW_WITH_CAUTION":
			result["POTENTIAL_RISK"] += row.Count
		case "NO_RISK", "ALLOW":
			result["NO_RISK"] += row.Count
		}
	}
	return result, nil
}

// GetTrademarkRecommendationCounts returns counts grouped by trademark_recommendation.
// For existing records without trademark_recommendation, derives from trademark_score.
func (d *Database) GetTrademarkRecommendationCounts(batchID uint) (map[string]int64, error) {
	result := map[string]int64{
		"YES_RISK":       0,
		"POTENTIAL_RISK": 0,
		"NO_RISK":        0,
	}

	// First try getting from trademark_recommendation field
	query := d.gorm.Model(&Evaluation{}).
		Select("COALESCE(NULLIF(trademark_recommendation, ''), CASE WHEN trademark_score >= 4 THEN 'YES_RISK' WHEN trademark_score >= 2 THEN 'POTENTIAL_RISK' ELSE 'NO_RISK' END) as recommendation, COUNT(*) as count").
		Group("recommendation")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	var rows []RecommendationCount
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		if row.Recommendation == "" {
			continue
		}
		switch row.Recommendation {
		case "YES_RISK":
			result["YES_RISK"] += row.Count
		case "POTENTIAL_RISK":
			result["POTENTIAL_RISK"] += row.Count
		case "NO_RISK":
			result["NO_RISK"] += row.Count
		}
	}
	return result, nil
}

// GetViceRecommendationCounts returns counts grouped by vice_recommendation.
// For existing records without vice_recommendation, derives from vice_score.
func (d *Database) GetViceRecommendationCounts(batchID uint) (map[string]int64, error) {
	result := map[string]int64{
		"YES_RISK":       0,
		"POTENTIAL_RISK": 0,
		"NO_RISK":        0,
	}

	// First try getting from vice_recommendation field, fall back to score
	query := d.gorm.Model(&Evaluation{}).
		Select("COALESCE(NULLIF(vice_recommendation, ''), CASE WHEN vice_score >= 4 THEN 'YES_RISK' WHEN vice_score >= 2 THEN 'POTENTIAL_RISK' ELSE 'NO_RISK' END) as recommendation, COUNT(*) as count").
		Group("recommendation")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	var rows []RecommendationCount
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		if row.Recommendation == "" {
			continue
		}
		switch row.Recommendation {
		case "YES_RISK":
			result["YES_RISK"] += row.Count
		case "POTENTIAL_RISK":
			result["POTENTIAL_RISK"] += row.Count
		case "NO_RISK":
			result["NO_RISK"] += row.Count
		}
	}
	return result, nil
}

// GetTrademarkScoreDistribution returns trademark score histogram (0-5).
func (d *Database) GetTrademarkScoreDistribution(batchID uint) ([]ScoreBucket, error) {
	query := d.gorm.Model(&Evaluation{}).
		Select("trademark_score as score, COUNT(*) as count").
		Group("trademark_score").
		Order("trademark_score ASC")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	var rows []ScoreBucket
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetViceScoreDistribution returns vice score histogram (0-5).
func (d *Database) GetViceScoreDistribution(batchID uint) ([]ScoreBucket, error) {
	query := d.gorm.Model(&Evaluation{}).
		Select("vice_score as score, COUNT(*) as count").
		Group("vice_score").
		Order("vice_score ASC")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	var rows []ScoreBucket
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetTopRisksByTrademark returns top N evaluations by trademark score.
func (d *Database) GetTopRisksByTrademark(batchID uint, limit int) ([]Evaluation, error) {
	query := d.gorm.Model(&Evaluation{}).
		Where("trademark_score > 0").
		Order("trademark_score DESC, vice_score DESC, id DESC")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	var rows []Evaluation
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetTopRisksByVice returns top N evaluations by vice score.
func (d *Database) GetTopRisksByVice(batchID uint, limit int) ([]Evaluation, error) {
	query := d.gorm.Model(&Evaluation{}).
		Where("vice_score > 0").
		Order("vice_score DESC, trademark_score DESC, id DESC")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	var rows []Evaluation
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetEvaluationsOverTime returns daily evaluation counts for the last N days.
func (d *Database) GetEvaluationsOverTime(batchID uint, days int) ([]TimeSeriesPoint, error) {
	if days <= 0 {
		days = 30
	}

	query := d.gorm.Model(&Evaluation{}).
		Select("DATE(created_at) as date, COUNT(*) as value").
		Where("created_at >= DATE('now', ?)", fmt.Sprintf("-%d days", days)).
		Group("DATE(created_at)").
		Order("date ASC")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	var rows []TimeSeriesPoint
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetAverageProcessingTime returns average processing time in milliseconds.
func (d *Database) GetAverageProcessingTime(batchID uint) (float64, error) {
	query := d.gorm.Model(&Evaluation{}).
		Select("AVG(processing_time_ms)")

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	var avg *float64
	if err := query.Scan(&avg).Error; err != nil {
		return 0, err
	}
	if avg == nil {
		return 0, nil
	}
	return *avg, nil
}

// CountEvaluations returns total evaluation count, optionally filtered by batch.
func (d *Database) CountEvaluations(batchID uint) (int64, error) {
	query := d.gorm.Model(&Evaluation{})

	if batchID > 0 {
		query = query.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetBatchSummaries returns aggregated statistics per batch.
func (d *Database) GetBatchSummaries(limit int) ([]BatchSummary, error) {
	if limit <= 0 {
		limit = 10
	}

	var batches []CSVBatch
	if err := d.gorm.Order("created_at DESC").Limit(limit).Find(&batches).Error; err != nil {
		return nil, err
	}

	summaries := make([]BatchSummary, 0, len(batches))
	for _, b := range batches {
		summary := BatchSummary{
			ID:               b.ID,
			Name:             b.Name,
			Owner:            b.Owner,
			TotalDomains:     b.UniqueDomains,
			ProcessedDomains: b.ProcessedDomains,
			CreatedAt:        b.CreatedAt,
			LastEvaluatedAt:  b.LastEvaluatedAt,
		}

		// Get legacy overall counts
		counts, err := d.GetRecommendationCounts(b.ID)
		if err == nil {
			summary.YesRiskCount = counts["YES_RISK"]
			summary.PotentialCount = counts["POTENTIAL_RISK"]
			summary.NoRiskCount = counts["NO_RISK"]
		}

		// Get trademark recommendation counts
		tmCounts, err := d.GetTrademarkRecommendationCounts(b.ID)
		if err == nil {
			summary.TrademarkYesRisk = tmCounts["YES_RISK"]
			summary.TrademarkPotentialRisk = tmCounts["POTENTIAL_RISK"]
			summary.TrademarkNoRisk = tmCounts["NO_RISK"]
		}

		// Get vice recommendation counts
		viceCounts, err := d.GetViceRecommendationCounts(b.ID)
		if err == nil {
			summary.ViceYesRisk = viceCounts["YES_RISK"]
			summary.VicePotentialRisk = viceCounts["POTENTIAL_RISK"]
			summary.ViceNoRisk = viceCounts["NO_RISK"]
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// ========================
// Override and Feedback Methods
// ========================

// GetEvaluationByID retrieves an evaluation by its ID.
func (d *Database) GetEvaluationByID(evaluationID uint) (*Evaluation, error) {
	var eval Evaluation
	if err := d.gorm.First(&eval, evaluationID).Error; err != nil {
		return nil, err
	}
	return &eval, nil
}

// GetEvaluationByDomain retrieves an evaluation by normalized domain.
func (d *Database) GetEvaluationByDomain(domainNormalized string) (*Evaluation, error) {
	var eval Evaluation
	if err := d.gorm.Where("domain_normalized = ?", domainNormalized).First(&eval).Error; err != nil {
		return nil, err
	}
	return &eval, nil
}

// CreateOverride creates a new evaluation override record.
func (d *Database) CreateOverride(override *EvaluationOverride) error {
	if override == nil {
		return errors.New("override is nil")
	}
	return d.gorm.Create(override).Error
}

// GetOverridesByEvaluation returns all overrides for a specific evaluation.
func (d *Database) GetOverridesByEvaluation(evaluationID uint) ([]EvaluationOverride, error) {
	var overrides []EvaluationOverride
	if err := d.gorm.Where("evaluation_id = ?", evaluationID).
		Order("created_at DESC").
		Find(&overrides).Error; err != nil {
		return nil, err
	}
	return overrides, nil
}

// GetOverrideByID retrieves an override by ID.
func (d *Database) GetOverrideByID(overrideID uint) (*EvaluationOverride, error) {
	var override EvaluationOverride
	if err := d.gorm.First(&override, overrideID).Error; err != nil {
		return nil, err
	}
	return &override, nil
}

// ListAllOverrides returns paginated override records, optionally filtered by user.
func (d *Database) ListAllOverrides(offset, limit int, user string) ([]EvaluationOverride, int64, error) {
	var total int64
	baseQuery := d.gorm.Model(&EvaluationOverride{})
	if user != "" {
		baseQuery = baseQuery.Where("overridden_by = ?", user)
	}
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	query := baseQuery.Order("created_at DESC")
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	var overrides []EvaluationOverride
	if err := query.Find(&overrides).Error; err != nil {
		return nil, 0, err
	}
	return overrides, total, nil
}

// UpdateOverrideFeedbackStatus marks an override as having feedback applied.
func (d *Database) UpdateOverrideFeedbackStatus(overrideID uint, embeddingID uint) error {
	return d.gorm.Model(&EvaluationOverride{}).
		Where("id = ?", overrideID).
		Updates(map[string]any{
			"feedback_applied": true,
			"embedding_id":     embeddingID,
		}).Error
}

// ApplyOverrideToEvaluation updates the evaluation with override values.
func (d *Database) ApplyOverrideToEvaluation(evaluationID uint, override *EvaluationOverride) error {
	if override == nil {
		return errors.New("override is nil")
	}
	now := time.Now()
	updates := map[string]any{
		"manual_override":  true,
		"last_override_at": &now,
	}
	// Handle trademark recommendation override
	if override.OverrideTrademarkRecommendation != "" {
		updates["trademark_recommendation"] = override.OverrideTrademarkRecommendation
	}
	if override.OverrideTrademarkScore != nil {
		updates["trademark_score"] = *override.OverrideTrademarkScore
	}
	// Handle vice recommendation override
	if override.OverrideViceRecommendation != "" {
		updates["vice_recommendation"] = override.OverrideViceRecommendation
	}
	if override.OverrideViceScore != nil {
		updates["vice_score"] = *override.OverrideViceScore
	}
	// Derive overall from the two (for legacy compatibility)
	tmRec := override.OverrideTrademarkRecommendation
	viceRec := override.OverrideViceRecommendation
	if tmRec != "" || viceRec != "" {
		// Calculate overall: YES_RISK if either is YES_RISK, else highest
		overall := "NO_RISK"
		if tmRec == "YES_RISK" || viceRec == "YES_RISK" {
			overall = "YES_RISK"
		} else if tmRec == "POTENTIAL_RISK" || viceRec == "POTENTIAL_RISK" {
			overall = "POTENTIAL_RISK"
		}
		updates["overall_recommendation"] = overall
	}
	if override.OverrideExplanation != "" {
		updates["explanation"] = override.OverrideExplanation
	}
	// Increment override count
	if err := d.gorm.Model(&Evaluation{}).Where("id = ?", evaluationID).
		UpdateColumn("override_count", gorm.Expr("COALESCE(override_count, 0) + 1")).Error; err != nil {
		return err
	}
	return d.gorm.Model(&Evaluation{}).Where("id = ?", evaluationID).Updates(updates).Error
}

// CreateFeedbackEmbedding stores a new feedback embedding.
func (d *Database) CreateFeedbackEmbedding(embedding *FeedbackEmbedding) error {
	if embedding == nil {
		return errors.New("embedding is nil")
	}
	return d.gorm.Create(embedding).Error
}

// GetFeedbackEmbedding retrieves a feedback embedding by ID.
func (d *Database) GetFeedbackEmbedding(id uint) (*FeedbackEmbedding, error) {
	var embedding FeedbackEmbedding
	if err := d.gorm.First(&embedding, id).Error; err != nil {
		return nil, err
	}
	return &embedding, nil
}

// GetAllEmbeddings returns all feedback embeddings for similarity search.
func (d *Database) GetAllEmbeddings() ([]FeedbackEmbedding, error) {
	var embeddings []FeedbackEmbedding
	if err := d.gorm.Find(&embeddings).Error; err != nil {
		return nil, err
	}
	return embeddings, nil
}

// ListFeedbackEmbeddings returns paginated feedback embeddings.
func (d *Database) ListFeedbackEmbeddings(offset, limit int) ([]FeedbackEmbedding, int64, error) {
	var total int64
	if err := d.gorm.Model(&FeedbackEmbedding{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	query := d.gorm.Model(&FeedbackEmbedding{}).Order("created_at DESC")
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	var embeddings []FeedbackEmbedding
	if err := query.Find(&embeddings).Error; err != nil {
		return nil, 0, err
	}
	return embeddings, total, nil
}

// MarkOverrideProcessed marks an override as having a feedback embedding created.
func (d *Database) MarkOverrideProcessed(overrideID, embeddingID uint) error {
	return d.gorm.Model(&EvaluationOverride{}).
		Where("id = ?", overrideID).
		Updates(map[string]any{
			"feedback_applied": true,
			"embedding_id":     embeddingID,
		}).Error
}

// IncrementEmbeddingRetrievalCount updates usage stats for retrieved embeddings.
func (d *Database) IncrementEmbeddingRetrievalCount(embeddingIDs []uint) error {
	if len(embeddingIDs) == 0 {
		return nil
	}
	now := time.Now()
	return d.gorm.Model(&FeedbackEmbedding{}).
		Where("id IN ?", embeddingIDs).
		Updates(map[string]any{
			"retrieval_count":   gorm.Expr("retrieval_count + 1"),
			"last_retrieved_at": &now,
		}).Error
}

// GetUnprocessedOverrides returns overrides that haven't been converted to embeddings.
func (d *Database) GetUnprocessedOverrides(limit int) ([]EvaluationOverride, error) {
	query := d.gorm.Where("feedback_applied = ?", false).Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var overrides []EvaluationOverride
	if err := query.Find(&overrides).Error; err != nil {
		return nil, err
	}
	return overrides, nil
}

// ========================
// Feedback Statistics
// ========================

// CorrectionPattern holds FROM->TO recommendation correction counts.
type CorrectionPattern struct {
	FromRecommendation string  `json:"from_recommendation"`
	ToRecommendation   string  `json:"to_recommendation"`
	Count              int     `json:"count"`
	Percentage         float64 `json:"percentage"`
}

// UserOverrideStat holds per-user override statistics.
type UserOverrideStat struct {
	User             string     `json:"user"`
	TotalOverrides   int        `json:"total_overrides"`
	MostCommonChange string     `json:"most_common_change"`
	LastOverrideAt   *time.Time `json:"last_override_at"`
}

// ConfidenceBucket holds accuracy per confidence range.
type ConfidenceBucket struct {
	ConfidenceMin   float64 `json:"confidence_min"`
	ConfidenceMax   float64 `json:"confidence_max"`
	TotalCount      int     `json:"total_count"`
	OverriddenCount int     `json:"overridden_count"`
	AccuracyPercent float64 `json:"accuracy_percent"`
}

// FeedbackStats holds comprehensive feedback analytics.
type FeedbackStats struct {
	TotalOverrides      int64               `json:"total_overrides"`
	TotalEvaluations    int64               `json:"total_evaluations"`
	OverrideRate        float64             `json:"override_rate"`
	AccuracyPercent     float64             `json:"accuracy_percent"`
	BlockToAllowRate    float64             `json:"block_to_allow_rate"`
	AllowToBlockRate    float64             `json:"allow_to_block_rate"`
	AvgScoreAdjustment  float64             `json:"avg_score_adjustment"`
	CorrectionPatterns  []CorrectionPattern `json:"correction_patterns"`
	UserStats           []UserOverrideStat  `json:"user_stats"`
	ConfidenceBuckets   []ConfidenceBucket  `json:"confidence_buckets"`
	OverridesOverTime   []TimeSeriesPoint   `json:"overrides_over_time"`
	AccuracyOverTime    []TimeSeriesPoint   `json:"accuracy_over_time"`
}

// GetFeedbackStats returns comprehensive feedback analytics.
func (d *Database) GetFeedbackStats(batchID uint) (*FeedbackStats, error) {
	stats := &FeedbackStats{}

	// Total evaluations
	evalQuery := d.gorm.Model(&Evaluation{})
	if batchID > 0 {
		evalQuery = evalQuery.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}
	if err := evalQuery.Count(&stats.TotalEvaluations).Error; err != nil {
		return nil, err
	}

	// Total overrides
	overrideQuery := d.gorm.Model(&EvaluationOverride{})
	if batchID > 0 {
		overrideQuery = overrideQuery.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}
	if err := overrideQuery.Count(&stats.TotalOverrides).Error; err != nil {
		return nil, err
	}

	// Calculate override rate and accuracy
	if stats.TotalEvaluations > 0 {
		stats.OverrideRate = (float64(stats.TotalOverrides) / float64(stats.TotalEvaluations)) * 100
		stats.AccuracyPercent = 100 - stats.OverrideRate
	}

	// Correction patterns
	var patterns []struct {
		OriginalRecommendation string
		OverrideRecommendation string
		Count                  int64
	}
	patternQuery := d.gorm.Model(&EvaluationOverride{}).
		Select("original_recommendation, override_recommendation, COUNT(*) as count").
		Group("original_recommendation, override_recommendation").
		Order("count DESC")
	if batchID > 0 {
		patternQuery = patternQuery.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}
	if err := patternQuery.Scan(&patterns).Error; err != nil {
		return nil, err
	}

	var blockToAllow, allowToBlock int64
	for _, p := range patterns {
		pct := float64(0)
		if stats.TotalOverrides > 0 {
			pct = (float64(p.Count) / float64(stats.TotalOverrides)) * 100
		}
		stats.CorrectionPatterns = append(stats.CorrectionPatterns, CorrectionPattern{
			FromRecommendation: p.OriginalRecommendation,
			ToRecommendation:   p.OverrideRecommendation,
			Count:              int(p.Count),
			Percentage:         pct,
		})
		// Track specific patterns
		if p.OriginalRecommendation == "BLOCK" && (p.OverrideRecommendation == "ALLOW" || p.OverrideRecommendation == "ALLOW_WITH_CAUTION") {
			blockToAllow += p.Count
		}
		if (p.OriginalRecommendation == "ALLOW" || p.OriginalRecommendation == "ALLOW_WITH_CAUTION") && p.OverrideRecommendation == "BLOCK" {
			allowToBlock += p.Count
		}
	}
	if stats.TotalOverrides > 0 {
		stats.BlockToAllowRate = float64(blockToAllow) / float64(stats.TotalOverrides) * 100
		stats.AllowToBlockRate = float64(allowToBlock) / float64(stats.TotalOverrides) * 100
	}

	// Average score adjustment
	var avgAdj *float64
	adjQuery := d.gorm.Model(&EvaluationOverride{}).
		Select("AVG(ABS(COALESCE(override_trademark_score, original_trademark_score) - original_trademark_score) + ABS(COALESCE(override_vice_score, original_vice_score) - original_vice_score))")
	if batchID > 0 {
		adjQuery = adjQuery.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}
	if err := adjQuery.Scan(&avgAdj).Error; err == nil && avgAdj != nil {
		stats.AvgScoreAdjustment = *avgAdj
	}

	// User stats with most common change
	var userStats []struct {
		OverriddenBy   string
		TotalOverrides int64
		LastOverrideAt string // SQLite returns datetime as string
	}
	userQuery := d.gorm.Model(&EvaluationOverride{}).
		Select("overridden_by, COUNT(*) as total_overrides, MAX(created_at) as last_override_at").
		Group("overridden_by").
		Order("total_overrides DESC").
		Limit(10)
	if batchID > 0 {
		userQuery = userQuery.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}
	if err := userQuery.Scan(&userStats).Error; err != nil {
		return nil, err
	}
	for _, u := range userStats {
		// Get most common change for this user
		var mostCommon struct {
			OriginalRecommendation string
			OverrideRecommendation string
		}
		d.gorm.Model(&EvaluationOverride{}).
			Select("original_recommendation, override_recommendation").
			Where("overridden_by = ?", u.OverriddenBy).
			Group("original_recommendation, override_recommendation").
			Order("COUNT(*) DESC").
			Limit(1).
			Scan(&mostCommon)

		change := ""
		if mostCommon.OriginalRecommendation != "" && mostCommon.OverrideRecommendation != "" {
			change = mostCommon.OriginalRecommendation + " → " + mostCommon.OverrideRecommendation
		}

		// Parse the datetime string from SQLite
		var lastAt *time.Time
		if u.LastOverrideAt != "" {
			if parsed, err := time.Parse("2006-01-02 15:04:05", u.LastOverrideAt); err == nil {
				lastAt = &parsed
			} else if parsed, err := time.Parse(time.RFC3339, u.LastOverrideAt); err == nil {
				lastAt = &parsed
			}
		}

		stats.UserStats = append(stats.UserStats, UserOverrideStat{
			User:             u.OverriddenBy,
			TotalOverrides:   int(u.TotalOverrides),
			MostCommonChange: change,
			LastOverrideAt:   lastAt,
		})
	}

	// Overrides over time (last 30 days)
	var overridesTime []struct {
		Date  string
		Count int64
	}
	timeQuery := d.gorm.Model(&EvaluationOverride{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at >= DATE('now', '-30 days')").
		Group("DATE(created_at)").
		Order("date ASC")
	if batchID > 0 {
		timeQuery = timeQuery.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}
	if err := timeQuery.Scan(&overridesTime).Error; err != nil {
		return nil, err
	}
	for _, t := range overridesTime {
		stats.OverridesOverTime = append(stats.OverridesOverTime, TimeSeriesPoint{
			Date:  t.Date,
			Value: float64(t.Count),
		})
	}

	// Confidence calibration buckets
	calibrationRanges := []struct {
		min, max float64
	}{
		{0.9, 1.0},
		{0.8, 0.9},
		{0.7, 0.8},
		{0.6, 0.7},
		{0.0, 0.6},
	}
	for _, r := range calibrationRanges {
		var totalCount, overriddenCount int64
		baseQuery := d.gorm.Model(&Evaluation{}).
			Where("trademark_confidence >= ? AND trademark_confidence < ?", r.min, r.max)
		if batchID > 0 {
			baseQuery = baseQuery.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
		}
		baseQuery.Count(&totalCount)
		baseQuery.Where("manual_override = ?", true).Count(&overriddenCount)

		accuracy := 100.0
		if totalCount > 0 {
			accuracy = (1 - float64(overriddenCount)/float64(totalCount)) * 100
		}
		stats.ConfidenceBuckets = append(stats.ConfidenceBuckets, ConfidenceBucket{
			ConfidenceMin:   r.min,
			ConfidenceMax:   r.max,
			TotalCount:      int(totalCount),
			OverriddenCount: int(overriddenCount),
			AccuracyPercent: accuracy,
		})
	}

	// Accuracy over time (last 30 days) - calculated from daily override rate
	var dailyStats []struct {
		Date        string
		TotalEvals  int64
		Overridden  int64
	}
	accuracyQuery := d.gorm.Model(&Evaluation{}).
		Select("DATE(created_at) as date, COUNT(*) as total_evals, SUM(CASE WHEN manual_override = 1 THEN 1 ELSE 0 END) as overridden").
		Where("created_at >= DATE('now', '-30 days')").
		Group("DATE(created_at)").
		Order("date ASC")
	if batchID > 0 {
		accuracyQuery = accuracyQuery.Where("domain_normalized IN (SELECT domain_normalized FROM domain_batches WHERE batch_id = ?)", batchID)
	}
	if err := accuracyQuery.Scan(&dailyStats).Error; err == nil {
		for _, s := range dailyStats {
			accuracy := 100.0
			if s.TotalEvals > 0 {
				accuracy = (1 - float64(s.Overridden)/float64(s.TotalEvals)) * 100
			}
			stats.AccuracyOverTime = append(stats.AccuracyOverTime, TimeSeriesPoint{
				Date:  s.Date,
				Value: accuracy,
			})
		}
	}

	return stats, nil
}

// CountOverrides returns total override count.
func (d *Database) CountOverrides() (int64, error) {
	var count int64
	if err := d.gorm.Model(&EvaluationOverride{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountFeedbackEmbeddings returns total feedback embedding count.
func (d *Database) CountFeedbackEmbeddings() (int64, error) {
	var count int64
	if err := d.gorm.Model(&FeedbackEmbedding{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CreateTrainingTerm adds a new AI training term.
func (d *Database) CreateTrainingTerm(term *AITrainingTerm) error {
	return d.gorm.Create(term).Error
}

// GetTrainingTerms returns all training terms grouped by classification.
func (d *Database) GetTrainingTerms() ([]AITrainingTerm, error) {
	var terms []AITrainingTerm
	if err := d.gorm.Order("classification ASC, term ASC").Find(&terms).Error; err != nil {
		return nil, err
	}
	return terms, nil
}

// GetTrainingTermsByClassification returns terms for a specific classification.
func (d *Database) GetTrainingTermsByClassification(classification string) ([]AITrainingTerm, error) {
	var terms []AITrainingTerm
	if err := d.gorm.Where("classification = ?", classification).Order("term ASC").Find(&terms).Error; err != nil {
		return nil, err
	}
	return terms, nil
}

// DeleteTrainingTerm removes a training term by ID.
func (d *Database) DeleteTrainingTerm(id uint) error {
	return d.gorm.Delete(&AITrainingTerm{}, id).Error
}

// ResetAllData clears all user data from the database (evaluations, batches, overrides, feedback).
// Keeps reference data like marks and commercial sales.
func (d *Database) ResetAllData() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Order matters due to foreign key constraints
	tables := []interface{}{
		&FeedbackEmbedding{},
		&EvaluationOverride{},
		&BatchRequest{},
		&DomainBatch{},
		&JobState{},
		&Evaluation{},
		&CSVBatch{},
		&AITrainingTerm{},
	}

	for _, table := range tables {
		if err := d.gorm.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(table).Error; err != nil {
			return fmt.Errorf("clear table %T: %w", table, err)
		}
	}

	// Reset auto-increment counters (SQLite specific, ignored on other DBs)
	d.gorm.Exec("DELETE FROM sqlite_sequence WHERE name IN ('feedback_embeddings', 'evaluation_overrides', 'batch_requests', 'domain_batches', 'job_states', 'evaluations', 'csv_batches', 'ai_training_terms')")

	return nil
}
