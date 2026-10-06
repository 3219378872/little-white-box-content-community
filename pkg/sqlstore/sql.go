// Package sqlstore executes explicit SQL using database/sql and sqlx mapping.
// SQL text and arguments are never emitted to application logs.
package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"esx/pkg/lifecycle"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	x "github.com/jmoiron/sqlx"
)

var ErrNotFound = sql.ErrNoRows

// mysqlDuplicateEntry is MySQL error 1062 (ER_DUP_ENTRY).
const mysqlDuplicateEntry = 1062

// IsDuplicateKey reports whether err is a MySQL unique-key violation, the
// signal stores use to turn a racing insert into "already exists".
func IsDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDuplicateEntry
}

type SqlConf struct{ DriverName, DataSource string }
type Session interface {
	ExecCtx(context.Context, string, ...any) (sql.Result, error)
	QueryRowCtx(context.Context, any, string, ...any) error
	QueryRowsCtx(context.Context, any, string, ...any) error
	PrepareCtx(context.Context, string) (StmtSession, error)
}
type SqlConn interface {
	Session
	RawDB() (*sql.DB, error)
	TransactCtx(context.Context, func(context.Context, Session) error) error
}
type StmtSession interface {
	Close() error
	ExecCtx(context.Context, ...any) (sql.Result, error)
	QueryRowCtx(context.Context, any, ...any) error
	QueryRowsCtx(context.Context, any, ...any) error
}
type conn struct {
	db  *x.DB
	ext x.ExtContext
	// afterCommit collects hooks registered inside TransactCtx; nil outside a transaction.
	afterCommit *[]func()
}
type statement struct{ s *x.Stmt }

func NewConn(c SqlConf) (SqlConn, error) {
	db, err := x.Open(c.DriverName, c.DataSource)
	if err != nil {
		return nil, err
	}
	lifecycle.TrackResource(db)
	db.SetMaxOpenConns(64)
	db.SetMaxIdleConns(16)
	db.SetConnMaxLifetime(10 * time.Minute)
	return &conn{db: db, ext: db}, nil
}
func NewMysql(dsn string) SqlConn {
	c, err := NewConn(SqlConf{DriverName: "mysql", DataSource: dsn})
	if err != nil {
		panic(err)
	}
	return c
}
func NewSqlConnFromDB(db *sql.DB) SqlConn     { v := x.NewDb(db, "mysql"); return &conn{db: v, ext: v} }
func NewSqlConnFromSession(s Session) SqlConn { return &sessionConn{Session: s} }
func (c *conn) RawDB() (*sql.DB, error)       { return c.db.DB, nil }
func (c *conn) ExecCtx(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return c.ext.ExecContext(ctx, q, args...)
}
func (c *conn) QueryRowCtx(ctx context.Context, v any, q string, args ...any) error {
	return x.GetContext(ctx, c.ext, v, q, args...)
}
func (c *conn) QueryRowsCtx(ctx context.Context, v any, q string, args ...any) error {
	return x.SelectContext(ctx, c.ext, v, q, args...)
}
func (c *conn) PrepareCtx(ctx context.Context, q string) (StmtSession, error) {
	p, ok := c.ext.(x.PreparerContext)
	if !ok {
		return nil, fmt.Errorf("SQL session cannot prepare statements")
	}
	s, err := x.PreparexContext(ctx, p, q)
	if err != nil {
		return nil, err
	}
	return &statement{s: s}, nil
}
func (c *conn) TransactCtx(ctx context.Context, fn func(context.Context, Session) error) error {
	tx, err := c.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var hooks []func()
	if err = fn(ctx, &conn{db: c.db, ext: tx, afterCommit: &hooks}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for _, hook := range hooks {
		hook()
	}
	return nil
}

// AfterCommit registers fn to run once the TransactCtx transaction owning
// session commits; a rollback discards it. It reports false when session is
// not such a transaction, in which case fn is not registered.
func AfterCommit(session Session, fn func()) bool {
	switch s := session.(type) {
	case *conn:
		if s.afterCommit == nil {
			return false
		}
		*s.afterCommit = append(*s.afterCommit, fn)
		return true
	case *sessionConn:
		return AfterCommit(s.Session, fn)
	}
	return false
}

type sessionConn struct{ Session }

func (c *sessionConn) RawDB() (*sql.DB, error) {
	return nil, fmt.Errorf("transaction session has no raw database")
}
func (c *sessionConn) TransactCtx(context.Context, func(context.Context, Session) error) error {
	return fmt.Errorf("nested transaction is not supported")
}
func (s *statement) Close() error { return s.s.Close() }
func (s *statement) ExecCtx(ctx context.Context, args ...any) (sql.Result, error) {
	return s.s.ExecContext(ctx, args...)
}
func (s *statement) QueryRowCtx(ctx context.Context, v any, args ...any) error {
	return s.s.GetContext(ctx, v, args...)
}
func (s *statement) QueryRowsCtx(ctx context.Context, v any, args ...any) error {
	return s.s.SelectContext(ctx, v, args...)
}
