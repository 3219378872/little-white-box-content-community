package idempotencyx

import (
	"context"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
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
