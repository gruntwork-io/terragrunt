package component

import (
	"sync"
)

// flushable is any writer that supports Flush() error.
type flushable interface {
	Flush() error
}

// unitOutputLocks provides per-unit locks for serializing flushes to the same writer.
var unitOutputLocks sync.Map // map[string]*sync.Mutex

func getUnitOutputLock(path string) *sync.Mutex {
	if mu, ok := unitOutputLocks.Load(path); ok {
		return mu.(*sync.Mutex)
	}

	newMu := &sync.Mutex{}
	actual, _ := unitOutputLocks.LoadOrStore(path, newMu)

	return actual.(*sync.Mutex)
}

// FlushOutput flushes buffer data to the output writer for this unit, if the writer supports it.
// This is safe to call even if Execution or the Writer is nil.
func FlushOutput(u *Unit) error {
	if u == nil || u.Execution == nil || u.Execution.TerragruntOptions == nil || u.Execution.TerragruntOptions.Writer == nil {
		return nil
	}

	writer, ok := u.Execution.TerragruntOptions.Writer.(flushable)
	if !ok {
		return nil
	}

	key := u.AbsolutePath()
	mu := getUnitOutputLock(key)

	mu.Lock()
	defer mu.Unlock()

	return writer.Flush()
}
