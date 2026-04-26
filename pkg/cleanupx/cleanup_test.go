package cleanupx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeromicro/go-zero/core/logx"
)

type recordingCloser struct {
	called bool
	err    error
}

func (c *recordingCloser) Close() error {
	c.called = true
	return c.err
}

func testLogger() logx.Logger {
	logx.Disable()
	return logx.WithContext(context.Background())
}

func TestCloseNilDoesNotPanic(t *testing.T) {
	Close(testLogger(), "nil resource", nil)
}

func TestCloseCallsCloser(t *testing.T) {
	closer := &recordingCloser{}

	Close(testLogger(), "temp file", closer)

	if !closer.called {
		t.Fatal("Close did not call closer")
	}
}

func TestCloseErrorDoesNotPanic(t *testing.T) {
	closer := &recordingCloser{err: errors.New("close failed")}

	Close(testLogger(), "temp file", closer)

	if !closer.called {
		t.Fatal("Close did not call closer")
	}
}

func TestShutdownNilDoesNotPanic(t *testing.T) {
	Shutdown(testLogger(), "nil shutdown", nil)
}

func TestShutdownCallsFunction(t *testing.T) {
	called := false

	Shutdown(testLogger(), "consumer", func() error {
		called = true
		return nil
	})

	if !called {
		t.Fatal("Shutdown did not call function")
	}
}

func TestShutdownErrorDoesNotPanic(t *testing.T) {
	called := false

	Shutdown(testLogger(), "consumer", func() error {
		called = true
		return errors.New("shutdown failed")
	})

	if !called {
		t.Fatal("Shutdown did not call function")
	}
}

func TestRemoveDeletesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup.txt")
	if err := os.WriteFile(path, []byte("cleanup"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	Remove(testLogger(), path)

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still exists after Remove, stat err=%v", err)
	}
}

func TestRemoveMissingFileDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.txt")

	Remove(testLogger(), path)
}

func TestRemoveEmptyPathDoesNotPanic(t *testing.T) {
	Remove(testLogger(), "")
}
