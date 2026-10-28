package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	drv "github.com/go-sql-driver/mysql"

	"github.com/lapp-coder/task-manager-service/internal/config"
	"github.com/lapp-coder/task-manager-service/internal/domain"
)

const errDuplicateEntry = 1062

func Open(ctx context.Context, cfg config.MySQL) (*sql.DB, error) {
	dsn := drv.NewConfig()
	dsn.Net = "tcp"
	dsn.Addr = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dsn.User = cfg.User
	dsn.Passwd = cfg.Password
	dsn.DBName = cfg.Database
	dsn.ParseTime = true
	dsn.Loc = time.UTC

	db, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("ping mysql: %w", err), db.Close())
	}
	return db, nil
}

type txKey struct{}

type TxManager struct {
	db *sql.DB
}

func NewTxManager(db *sql.DB) *TxManager {
	return &TxManager{db: db}
}

func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func conn(ctx context.Context, db *sql.DB) querier {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return db
}

type scanner interface {
	Scan(dest ...any) error
}

func queryAll[T any](ctx context.Context, q querier, scan func(scanner) (T, error), query string, args ...any) ([]T, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return items, nil
}

func insert(ctx context.Context, q querier, query string, args ...any) (int64, error) {
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func page[T any](items []T, req domain.PageRequest, id func(T) int64) domain.Page[T] {
	if len(items) <= req.Limit {
		return domain.Page[T]{Items: items}
	}
	items = items[:req.Limit]
	return domain.Page[T]{Items: items, NextCursor: id(items[len(items)-1])}
}

func isDuplicate(err error) bool {
	var me *drv.MySQLError
	return errors.As(err, &me) && me.Number == errDuplicateEntry
}

func nullID(id int64) sql.NullInt64 {
	return sql.NullInt64{Int64: id, Valid: id != 0}
}

func nullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: !t.IsZero()}
}
