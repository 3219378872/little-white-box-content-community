// Package sqlstore executes explicit SQL using database/sql and sqlx mapping.
// SQL text and arguments are never emitted to application logs.
package sqlstore

import (
	"context"
	"database/sql"
	"esx/pkg/lifecycle"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	x "github.com/jmoiron/sqlx"
)

var ErrNotFound = sql.ErrNoRows

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
	if err = fn(ctx, &conn{db: c.db, ext: tx}); err != nil {
		return err
	}
	return tx.Commit()
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
