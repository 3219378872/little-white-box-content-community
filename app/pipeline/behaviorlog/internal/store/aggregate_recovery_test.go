package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type aggregateSQLDriver struct{ state *aggregateSQLState }
type aggregateSQLState struct {
	write, read string
	args        []driver.NamedValue
	writeErr    error
}

func (d aggregateSQLDriver) Driver() driver.Driver            { return d }
func (d aggregateSQLDriver) Open(string) (driver.Conn, error) { return d.Connect(context.Background()) }
func (d aggregateSQLDriver) Connect(context.Context) (driver.Conn, error) {
	return &aggregateSQLConn{state: d.state}, nil
}

type aggregateSQLConn struct{ state *aggregateSQLState }

func (c *aggregateSQLConn) Close() error { return nil }
func (c *aggregateSQLConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *aggregateSQLConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c *aggregateSQLConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	c.state.write = q
	c.state.args = args
	return driver.RowsAffected(1), c.state.writeErr
}
func (c *aggregateSQLConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.read = q
	return &aggregateSQLRows{}, nil
}

type aggregateSQLRows struct{ done bool }

func (r *aggregateSQLRows) Columns() []string { return []string{"count"} }
func (r *aggregateSQLRows) Close() error      { return nil }
func (r *aggregateSQLRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = int64(2)
	return nil
}
func TestAggregateAdapterReadsCanonicalFactsAndZerosObsoleteGroups(t *testing.T) {
	state := &aggregateSQLState{}
	db := sql.OpenDB(aggregateSQLDriver{state: state})
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)
	count, err := NewClickHouseStore(db).AggregateDaily(context.Background(), from, to)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.Contains(t, state.write, "FROM xbh_analytics.behavior_facts")
	require.Contains(t, state.write, "UNION ALL")
	require.Contains(t, state.write, "FROM xbh_analytics.daily_aggregates FINAL")
	require.Contains(t, state.write, "toUInt64(0) AS cnt")
	require.Contains(t, state.read, "AND cnt > 0")
	require.Len(t, state.args, 4)
	require.Equal(t, state.args[0].Value, state.args[2].Value)
	require.Equal(t, state.args[1].Value, state.args[3].Value)
	state.writeErr = errors.New("canonical schema unavailable")
	state.read = ""
	_, err = NewClickHouseStore(db).AggregateDaily(context.Background(), from, to)
	require.Error(t, err)
	require.Empty(t, state.read)
}
