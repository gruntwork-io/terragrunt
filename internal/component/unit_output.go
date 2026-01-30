package component

import (
	"fmt"
	"io"
	"sync"
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

// FlushOutput flushes buffer data to the output writer for this unit, if the writer supports it.
// This is safe to call even if Execution or the Writer is nil.
func FlushOutput(u *Unit) error {
	if u == nil || u.Execution == nil || u.Execution.TerragruntOptions == nil || u.Execution.TerragruntOptions.Writer == nil {
		return nil
	}

	writer, ok := u.Execution.TerragruntOptions.Writer.(flusher)
	if !ok {
		return nil
	}

	// Use parent writer's address as lock key to serialize flushes to same parent.
	// Falls back to unit path for writers without parentWriterProvider.
	key := u.AbsolutePath()
	if pwp, ok := u.Execution.TerragruntOptions.Writer.(parentWriterProvider); ok {
		key = fmt.Sprintf("%p", pwp.ParentWriter())
	}

	mu := unitOutputLock(key)

	mu.Lock()
	defer mu.Unlock()

	return writer.Flush()
}
