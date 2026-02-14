package component

import (
	"fmt"
	"io"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// flusher is any writer that supports Flush() error.
type flusher interface {
	Flush() error
}

// parentWriterProvider is any writer that can provide its underlying parent writer.
// This is used to create writer-based locks that serialize flushes to the same parent.
type parentWriterProvider interface {
	ParentWriter() io.Writer
}

// unitOutputLocks provides locks for serializing flushes to the same parent writer.
// The key is the parent writer's address (via fmt.Sprintf("%p", writer)).
var unitOutputLocks sync.Map // map[string]*sync.Mutex

func unitOutputLock(key string) *sync.Mutex {
	if mu, ok := unitOutputLocks.Load(key); ok {
		return mu.(*sync.Mutex)
	}

	newMu := &sync.Mutex{}

	actual, loaded := unitOutputLocks.LoadOrStore(key, newMu)
	if loaded {
		return actual.(*sync.Mutex)
	}

	return newMu
}

// FlushOutput flushes buffer data to the given writer for this unit, if the writer supports it.
// This is safe to call even if u or w is nil.
func FlushOutput(l log.Logger, u *Unit, w io.Writer) error {
	if u == nil || w == nil {
		return nil
	}

	writer, ok := w.(flusher)
	if !ok {
		return nil
	}

	// Use parent writer's address as lock key to serialize flushes to same parent.
	// Falls back to unit path for writers without parentWriterProvider.
	key := u.AbsolutePath(l)
	if pwp, ok := w.(parentWriterProvider); ok {
		key = fmt.Sprintf("%p", pwp.ParentWriter())
	}

	mu := unitOutputLock(key)

	mu.Lock()
	defer mu.Unlock()

	return writer.Flush()
}
