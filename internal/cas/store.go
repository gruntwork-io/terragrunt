package cas

import (
	"path/filepath"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// Store manages one content-addressed directory of the CAS.
//
// Every writer publishes an object by renaming a uniquely named temp
// file onto its hash-addressed path, so two writers of one hash leave
// the same result and racing costs only duplicated work. [Store.Lock]
// spares that work within a process; across processes writers race on
// the rename, so the store needs no per-object lock files.
type Store struct {
	path string
}

// objectLocks serializes in-process writers per object path. It is
// process-wide rather than a Store field because Terragrunt builds a
// fresh [CAS] per download, and instances sharing a store path must
// share the lock for the dedupe to hold.
var objectLocks = newKeyedLocks()

// NewStore creates a new Store rooted at path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Path returns the current store path.
func (s *Store) Path() string {
	return s.path
}

// NeedsWrite checks if a given hash needs to be stored.
func (s *Store) NeedsWrite(v *venv.Venv, hash string) bool {
	return !s.hasContent(v, s.objectPath(hash))
}

// Lock blocks until no other goroutine in this process is writing the
// object at hash and returns the function that releases it, which must
// be called exactly once. It offers no protection against other
// processes.
func (s *Store) Lock(hash string) (unlock func()) {
	return objectLocks.lock(s.objectPath(hash))
}

// EnsureWithWait reports whether the object at hash still has to be
// written and, when it does, holds the in-process writer lock for it.
// The existence check is repeated once the lock is held, so a caller
// that waited on another writer of the same hash learns that the write
// is already done instead of repeating it.
//
// unlock must always be called; it releases the lock when needsWrite
// is true and does nothing otherwise.
func (s *Store) EnsureWithWait(v *venv.Venv, hash string) (needsWrite bool, unlock func()) {
	if !s.NeedsWrite(v, hash) {
		return false, func() {}
	}

	unlock = s.Lock(hash)

	if !s.NeedsWrite(v, hash) {
		unlock()

		return false, func() {}
	}

	return true, unlock
}

func (s *Store) objectPath(hash string) string {
	return filepath.Join(s.path, hash[:2], hash)
}

func (s *Store) hasContent(v *venv.Venv, path string) bool {
	_, err := v.FS.Stat(path)

	return err == nil
}

// keyedLocks hands out one mutex per key and forgets a key once no
// goroutine holds or waits on it, so the table grows with in-flight
// writes rather than with every object the process has ever stored.
type keyedLocks struct {
	entries map[string]*keyedLock
	mu      sync.Mutex
}

// keyedLock is one entry of [keyedLocks].
type keyedLock struct {
	mu   sync.Mutex
	refs int
}

func newKeyedLocks() *keyedLocks {
	return &keyedLocks{entries: make(map[string]*keyedLock)}
}

// lock blocks until key is free and returns the function that releases
// it.
func (kl *keyedLocks) lock(key string) func() {
	kl.mu.Lock()

	entry, ok := kl.entries[key]
	if !ok {
		entry = &keyedLock{}
		kl.entries[key] = entry
	}

	// Counting this goroutine in before the table lock goes keeps a
	// releasing peer from dropping the entry it is about to block on.
	// Two callers would otherwise hold different locks for one key.
	entry.refs++

	kl.mu.Unlock()

	entry.mu.Lock()

	return func() {
		entry.mu.Unlock()

		kl.mu.Lock()
		defer kl.mu.Unlock()

		entry.refs--

		if entry.refs == 0 {
			delete(kl.entries, key)
		}
	}
}
