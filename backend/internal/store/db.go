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
	if err := db.AutoMigrate(&Mark{}, &Domain{}, &Evaluation{}, &CommercialSale{}, &PopularMark{}, &CSVBatch{}, &BatchRequest{}, &DomainBatch{}, &JobState{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		logrus.WithError(err).Warn("enable WAL mode")
	}
	if err := db.Exec("PRAGMA synchronous=NORMAL").Error; err != nil {
		logrus.WithError(err).Warn("set synchronous pragma")
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
	columns := []string{
		"trademark_score",
		"trademark_type",
		"matched_trademark",
		"trademark_confidence",
		"vice_score",
		"vice_categories_json",
		"vice_confidence",
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
		"CREATE INDEX IF NOT EXISTS idx_domain_batches_batch_domain_normalized ON domain_batches(batch_id, domain_normalized)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_evaluations_domain_normalized ON evaluations(domain_normalized)",
		"CREATE INDEX IF NOT EXISTS idx_evaluations_trademark_score ON evaluations(trademark_score)",
		"CREATE INDEX IF NOT EXISTS idx_evaluations_vice_score ON evaluations(vice_score)",
		"CREATE INDEX IF NOT EXISTS idx_marks_mark_normalized ON marks(mark_normalized)",
		"CREATE INDEX IF NOT EXISTS idx_marks_mark_no_spaces ON marks(mark_no_spaces)",
		"CREATE INDEX IF NOT EXISTS idx_marks_owner ON marks(owner)",
		"CREATE INDEX IF NOT EXISTS idx_job_states_status_updated ON job_states(status, updated_at)",
		"CREATE INDEX IF NOT EXISTS idx_job_states_batch ON job_states(batch_id)",
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
