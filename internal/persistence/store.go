package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"seed-vigor-workbench/internal/domain"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = "seed-vigor.db"
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	store := &Store{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) initialize(ctx context.Context) error {
	pragmas := []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"}
	for _, statement := range pragmas {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("配置 SQLite: %w", err)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range migrations {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("执行 schema 迁移: %w", err)
		}
	}
	var count, version int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(version), 0) FROM schema_meta`).Scan(&count, &version); err != nil {
		return err
	}
	if count == 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_meta(version) VALUES(?)`, schemaVersion); err != nil {
			return err
		}
	} else if version != schemaVersion {
		return fmt.Errorf("不支持的 schema 版本 %d，当前程序要求 %d", version, schemaVersion)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	var integrity string
	if err := s.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("SQLite 完整性检查失败: %s", integrity)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Update(ctx context.Context, id string, expected int64, action, actor string, details map[string]any, change func(*domain.GerminationAssay) error) (*domain.GerminationAssay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	assay, err := loadAssay(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if assay.Revision != expected {
		return nil, domain.ConflictError{Expected: expected, Current: assay.Revision}
	}
	if assay.State.IsImmutable() {
		return nil, domain.ValidationError{Field: "state", Message: "归档批次不可修改"}
	}
	if err := change(assay); err != nil {
		return nil, err
	}
	assay.Revision++
	assay.UpdatedAt = time.Now().UTC()
	if action != "" {
		event := domain.AuditEvent{ID: newID("evt"), AssayID: id, Revision: assay.Revision, Action: action, Actor: actor, Details: details, CreatedAt: assay.UpdatedAt}
		assay.AuditTrail = append(assay.AuditTrail, event)
	}
	if err := saveAssay(ctx, tx, assay); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: assays.laboratory_batch_no") {
			return nil, domain.ValidationErrors{Issues: []domain.ValidationError{{Field: "sample_accession", Message: "同一检验批次号下样本标识重复"}}}
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return assay, nil
}
