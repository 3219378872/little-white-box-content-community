package idempotencyx

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	sqlx "esx/pkg/sqlstore"
)

type recordingQuerier struct {
	query string
}

func (q *recordingQuerier) QueryRowCtx(_ context.Context, _ any, query string, _ ...any) error {
	q.query = query
	return sqlx.ErrNotFound
}

func TestFindIdempotencySessionUsesCurrentReadForDuplicateResolution(t *testing.T) {
	querier := &recordingQuerier{}
	_, _, err := findIdempotencySession(context.Background(), querier, "post:create", 7, "same-key", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(querier.query, " FOR UPDATE") {
		t.Fatalf("duplicate resolution query = %q, want an InnoDB current read", querier.query)
	}

	_, _, err = findIdempotencySession(context.Background(), querier, "post:create", 7, "same-key", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(querier.query, "FOR UPDATE") {
		t.Fatalf("initial lookup query = %q, want a non-locking consistent read", querier.query)
	}
}

type resolverSession struct {
	sqlx.Session
	stored    storedIdempotency
	queries   int
	duplicate bool
	inserted  bool
}

func (s *resolverSession) QueryRowCtx(_ context.Context, value any, _ string, _ ...any) error {
	s.queries++
	if s.duplicate && s.queries == 1 {
		return sqlx.ErrNotFound
	}
	*value.(*storedIdempotency) = s.stored
	return nil
}
func (s *resolverSession) ExecCtx(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	s.inserted = true
	return nil, &mysql.MySQLError{Number: 1062}
}
func TestResolveIdempotencyVersionedAndLegacyReplay(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		for _, stored := range []string{"current", "safe-legacy", "ambiguous-legacy"} {
			session := &resolverSession{stored: storedIdempotency{ResourceID: 42, CommandHash: stored}, duplicate: duplicate}
			rec := IdempotencyRecord{Scope: "post:create", UserID: 1, Key: "key", CommandHash: "current", LegacyCommandHash: "safe-legacy"}
			id, created, err := ResolveIdempotencySession(context.Background(), session, rec, 9, 9)
			if stored == "ambiguous-legacy" {
				if !errors.Is(err, ErrIdempotencyConflict) || id != 0 || created {
					t.Fatalf("unexpected conflict result %d %v %v", id, created, err)
				}
			} else if err != nil || id != 42 || created {
				t.Fatalf("unexpected replay result %d %v %v", id, created, err)
			}
			if session.inserted != duplicate {
				t.Fatalf("unexpected insert: %v", session.inserted)
			}
		}
	}
}
func TestVersionedCommandHashIsLengthDelimited(t *testing.T) {
	if CommandHash("a\x00b", "c") != CommandHash("a", "b\x00c") {
		t.Fatal("fixture must collide under legacy encoding")
	}
	if VersionedCommandHash("v2", "a\x00b", "c") == VersionedCommandHash("v2", "a", "b\x00c") {
		t.Fatal("field boundary collision")
	}
	if VersionedCommandHash("v2", "a") == VersionedCommandHash("v3", "a") {
		t.Fatal("version collision")
	}
}
