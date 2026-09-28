package lifecycle

import (
	"io"
	"sync"
)

// TrackResource registers resources created by process-level infrastructure.
// Entrypoints defer CloseResources before starting servers/workers, so their
// own drain and shutdown defers run before these shared clients are closed.
var resources struct {
	sync.Mutex
	closers []io.Closer
}

func TrackResource(closer io.Closer) {
	resources.Lock()
	defer resources.Unlock()
	resources.closers = append(resources.closers, closer)
}
func CloseResources() {
	resources.Lock()
	closers := resources.closers
	resources.closers = nil
	resources.Unlock()
	for i := len(closers) - 1; i >= 0; i-- {
		_ = closers[i].Close()
	}
}

type closeResourceFunc func()

func (f closeResourceFunc) Close() error { f(); return nil }
