package sqlstore

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsDuplicateKey(t *testing.T) {
	assert.True(t, IsDuplicateKey(&mysql.MySQLError{Number: 1062}))
	assert.True(t, IsDuplicateKey(fmt.Errorf("insert: %w", &mysql.MySQLError{Number: 1062})))
	assert.False(t, IsDuplicateKey(&mysql.MySQLError{Number: 1213}))
	assert.False(t, IsDuplicateKey(sql.ErrNoRows))
	assert.False(t, IsDuplicateKey(nil))
}

func TestAfterCommitRejectsSessionsOutsideTransactCtx(t *testing.T) {
	// sql.Open does not dial, so no database is needed to build a plain conn.
	db, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:1)/none")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	plain := NewSqlConnFromDB(db)

	assert.False(t, AfterCommit(plain, func() {}))
	assert.False(t, AfterCommit(NewSqlConnFromSession(plain), func() {}))
	assert.False(t, AfterCommit(nil, func() {}))
}
