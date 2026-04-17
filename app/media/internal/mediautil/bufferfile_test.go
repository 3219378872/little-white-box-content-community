package mediautil

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTempSink_WriteWithinLimit(t *testing.T) {
	sink, err := NewTempSink("", 100)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sink.Close() })

	n, err := sink.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, int64(5), sink.Size())

	data, err := os.ReadFile(sink.Path())
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestTempSink_WriteExceedsLimit(t *testing.T) {
	sink, err := NewTempSink("", 4)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sink.Close() })

	_, err = sink.Write([]byte("abcd"))
	require.NoError(t, err)

	_, err = sink.Write([]byte("e"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSizeExceeded))
}

func TestTempSink_CloseRemovesFile(t *testing.T) {
	sink, err := NewTempSink("", 100)
	require.NoError(t, err)
	path := sink.Path()

	_, err = sink.Write([]byte("hello"))
	require.NoError(t, err)

	require.NoError(t, sink.Close())

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "文件应被 Close 删除")
}

func TestTempSink_CloseIdempotent(t *testing.T) {
	sink, err := NewTempSink("", 100)
	require.NoError(t, err)

	require.NoError(t, sink.Close())
	require.NoError(t, sink.Close(), "二次 Close 不应报错")
}
