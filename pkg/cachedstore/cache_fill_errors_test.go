package cachedstore

import (
	"context"
	"errors"
	"testing"

	"esx/pkg/sqlstore"

	"github.com/stretchr/testify/require"
)

func TestCacheErrorsFailClosedForFill(t *testing.T) {
	for _, tc := range []struct {
		command string
		after   bool
	}{
		{"GET", false}, {"SET", false}, {"SET", true}, {"EVAL", false}, {"EVAL", true},
	} {
		t.Run(tc.command+map[bool]string{false: "/before", true: "/after"}[tc.after], func(t *testing.T) {
			c, s := newCacheFixture(t)
			s.Fail(tc.command, tc.after)
			var row cacheRow
			require.NoError(t, c.QueryRowCtx(context.Background(), &row, "row:7", rowQuery(1, nil)))
			require.Equal(t, 1, row.Status, "cache errors must not replace the SQL result")
			value, _ := s.Value(Prefix + "row:7")
			if tc.command == "EVAL" && tc.after {
				// Lost fill ACK may leave a valid entry, but successful DEL still
				// removes it and no retry can republish that payload.
				require.Contains(t, value, `"Status":1`)
				require.NoError(t, c.DelCacheCtx(context.Background(), "row:7"))
			} else {
				require.NotContains(t, value, `"Status":1`)
			}
			if tc.command != "EVAL" {
				require.Zero(t, s.Calls("EVAL"), "unconfirmed reservations cannot fill")
			}
		})
	}
}

func TestFailedReadReleasesOnlyItsOwnReservation(t *testing.T) {
	c, s := newCacheFixture(t)
	failure := errors.New("SQL unavailable")
	var row cacheRow
	require.ErrorIs(t, c.QueryRowCtx(context.Background(), &row, "row:7", rowQuery(0, failure)), failure)
	value, _ := s.Value(Prefix + "row:7")
	require.Empty(t, value)
	require.NoError(t, c.QueryRowCtx(context.Background(), &row, "row:7", rowQuery(1, nil)))
}

func TestInvalidPayloadAndDisabledTTLDoNotRemainReserved(t *testing.T) {
	c, s := newCacheFixture(t)
	ctx := context.Background()
	unsupported := make(chan int)
	require.NoError(t, c.QueryRowCtx(ctx, &unsupported, "invalid", func(context.Context, sqlstore.SqlConn, any) error { return nil }))
	value, _ := s.Value(Prefix + "invalid")
	require.Empty(t, value)
	c.options.TTLSeconds = 0
	var row cacheRow
	require.NoError(t, c.QueryRowCtx(ctx, &row, "disabled", rowQuery(1, nil)))
	value, _ = s.Value(Prefix + "disabled")
	require.Empty(t, value)
}

func TestInvalidationFailureIsNotAConsistencyGuarantee(t *testing.T) {
	c, s := newCacheFixture(t)
	ctx := context.Background()
	var row cacheRow
	require.NoError(t, c.QueryRowCtx(ctx, &row, "row:7", rowQuery(1, nil)))
	s.Fail("DEL", false)
	require.Error(t, c.DelCacheCtx(ctx, "row:7"))
	require.NoError(t, c.QueryRowCtx(ctx, &row, "row:7", rowQuery(0, nil)))
	require.Equal(t, 1, row.Status, "authorization-sensitive callers must bypass this disposable cache")
}

func TestReservationCannotOverwriteExistingValue(t *testing.T) {
	c, s := newCacheFixture(t)
	for _, value := range []string{"fill:another-owner", `{"Missing":true}`} {
		s.Set(Prefix+"row:7", value, 12)
		require.Empty(t, c.reserveFill(context.Background(), Prefix+"row:7"))
		got, ttl := s.Value(Prefix + "row:7")
		require.Equal(t, value, got)
		require.Equal(t, 12, ttl)
	}
}
