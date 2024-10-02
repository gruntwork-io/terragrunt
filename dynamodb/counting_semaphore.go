package dynamodb

type empty struct{}
type CountingSemaphore chan empty

// NewCountingSemaphore is a bare-bones counting semaphore implementation
// based on: http://www.golangpatterns.info/concurrency/semaphores
func NewCountingSemaphore(size int) CountingSemaphore {
	return make(CountingSemaphore, size)
}

func (semaphore CountingSemaphore) Acquire() {
	semaphore <- empty{}
}

func (semaphore CountingSemaphore) Release() {
	<-semaphore
}
