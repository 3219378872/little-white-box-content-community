package mediautil

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

// ErrSizeExceeded 写入量超过 limit 时返回。
var ErrSizeExceeded = errors.New("media: size limit exceeded")

// TempSink 是一个限大小的临时文件写入器。
// Close 会关闭文件句柄并删除文件，未关闭时文件会遗留在 TempDir。
type TempSink struct {
	file    *os.File
	path    string
	limit   int64
	written int64
	once    sync.Once
	closed  bool
}

// NewTempSink 在 dir 下创建临时文件；dir 为空时使用 os.TempDir()。
func NewTempSink(dir string, limit int64) (*TempSink, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("media: invalid limit %d", limit)
	}
	f, err := os.CreateTemp(dir, "media-*.bin")
	if err != nil {
		return nil, fmt.Errorf("media: create temp file: %w", err)
	}
	return &TempSink{file: f, path: f.Name(), limit: limit}, nil
}

// Write 实现 io.Writer；写入后累计长度超过 limit 返回 ErrSizeExceeded。
func (t *TempSink) Write(p []byte) (int, error) {
	if t.written+int64(len(p)) > t.limit {
		return 0, ErrSizeExceeded
	}
	n, err := t.file.Write(p)
	t.written += int64(n)
	return n, err
}

// Path 返回临时文件绝对路径（Close 前有效）。
func (t *TempSink) Path() string { return t.path }

// Size 返回累计写入字节数。
func (t *TempSink) Size() int64 { return t.written }

// Close 关闭文件并删除磁盘文件；幂等。
func (t *TempSink) Close() error {
	var err error
	t.once.Do(func() {
		closeErr := t.file.Close()
		removeErr := os.Remove(t.path)
		t.closed = true
		if closeErr != nil {
			err = closeErr
			return
		}
		if removeErr != nil && !os.IsNotExist(removeErr) {
			err = removeErr
		}
	})
	return err
}
