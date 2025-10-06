package store

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// PopularMarks aggregates mark frequencies directly from the marks table. It groups by
// normalized token (mark without spaces, lowercase) and returns the most common entries.
func (d *Database) PopularMarks(limit int, minCount int) ([]PopularMark, error) {
	if d == nil {
		return nil, errors.New("database is nil")
	}
	if limit <= 0 {
		limit = 500000
	}
	if minCount <= 0 {
		minCount = 2
	}

	var results []PopularMark
	query := d.gorm.Table("marks").
		Select("LOWER(mark_no_spaces) AS normalized, MAX(mark) AS mark, COUNT(*) AS total").
		Group("LOWER(mark_no_spaces)").
		Having("COUNT(*) >= ?", minCount).
		Order("total DESC").
		Limit(limit)

	if err := query.Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("popular marks: %w", err)
	}
	return results, nil
}

// ReplacePopularMarks atomically swaps the popular_marks table with the provided slice.
func (d *Database) ReplacePopularMarks(marks []PopularMark) error {
	if d == nil {
		return errors.New("database is nil")
	}
	return d.gorm.Transaction(func(tx *gorm.DB) error {
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&PopularMark{}).Error; err != nil {
			return err
		}
		if len(marks) == 0 {
			return nil
		}
		// Batch insert to avoid SQLite variable limit (999)
		const batchSize = 250
		for start := 0; start < len(marks); start += batchSize {
			end := start + batchSize
			if end > len(marks) {
				end = len(marks)
			}
			batch := marks[start:end]
			if err := tx.CreateInBatches(batch, batchSize).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListPopularMarks returns popular mark rows ordered by frequency.
func (d *Database) ListPopularMarks(limit int) ([]PopularMark, error) {
	if d == nil {
		return nil, errors.New("database is nil")
	}
	query := d.gorm.Model(&PopularMark{}).Order("total DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []PopularMark
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
