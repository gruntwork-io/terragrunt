package s3

type empty struct{}

// CountingSemaphore limits how many holders run at once to its buffer size.
type CountingSemaphore chan empty

// NewCountingSemaphore is a bare-bones counting semaphore implementation
// based on: http://www.golangpatterns.info/concurrency/semaphores
func NewCountingSemaphore(size int) CountingSemaphore {
	return make(CountingSemaphore, size)
}

// Acquire takes a permit, blocking while all permits are held.
func (semaphore CountingSemaphore) Acquire() {
	semaphore <- empty{}
}

// Release returns a permit taken by [CountingSemaphore.Acquire].
//
// Blocks while no permit is held.
func (semaphore CountingSemaphore) Release() {
	<-semaphore
}
