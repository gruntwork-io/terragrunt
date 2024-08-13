//nolint:govet
package dynamodb_test

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/dynamodb"
)

func TestCountingSemaphoreHappyPath(t *testing.T) {
	t.Parallel()

	semaphore := dynamodb.NewCountingSemaphore(1)
	semaphore.Acquire()
	semaphore.Release()
}

// This method tries to verify our counting semaphore works. It does this by creating a counting semaphore of size N
// and then firing up M >> N goroutines that all try to Acquire the semaphore. As each goroutine executes, it uses an
// atomic increment operation to record how many goroutines are running simultaneously. We check the number of running
// goroutines to ensure that it goes up to N, but does not exceed it.
func TestCountingSemaphoreConcurrency(t *testing.T) {
	t.Parallel()

	permits := 10
	goroutines := 100
	semaphore := dynamodb.NewCountingSemaphore(permits)

	var goRoutinesExecutingSimultaneously uint32
	var waitForAllGoRoutinesToFinish sync.WaitGroup

	endGoRoutine := func() {
		// Decrement the number of running goroutines. Note that decrementing an unsigned int is a bit odd.
		// This is copied from the docs: https://golang.org/pkg/sync/atomic/#AddUint32
		atomic.AddUint32(&goRoutinesExecutingSimultaneously, ^uint32(0))

		semaphore.Release()
		waitForAllGoRoutinesToFinish.Done()
	}

	runGoRoutine := func() {
		defer endGoRoutine()
		semaphore.Acquire()

		// Increment the total number of running goroutines
		totalGoRoutinesExecutingSimultaneously := atomic.AddUint32(&goRoutinesExecutingSimultaneously, 1)

		if totalGoRoutinesExecutingSimultaneously > uint32(permits) {
			t.Fatalf("The semaphore was only supposed to allow %d goroutines to run simultaneously, but has allowed %d", permits, totalGoRoutinesExecutingSimultaneously)
		}

		// Sleep for a random amount of time to represent this goroutine doing work
		randomSleepTime := rand.Intn(100)
		time.Sleep(time.Duration(randomSleepTime) * time.Millisecond)
	}

	// Fire up a whole bunch of goroutines that will all try to acquire the semaphore at the same time
	for i := 0; i < goroutines; i++ {
		waitForAllGoRoutinesToFinish.Add(1)
		go runGoRoutine()
	}

	waitForAllGoRoutinesToFinish.Wait()
}
