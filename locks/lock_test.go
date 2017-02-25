package locks

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"testing"
)

// A mock lock that performs a No Op for every operation
type NoopLock struct{}

func (lock NoopLock) AcquireLock(terragruntOptions *options.TerragruntOptions) error { return nil }
func (lock NoopLock) ReleaseLock(terragruntOptions *options.TerragruntOptions) error { return nil }
func (lock NoopLock) String() string                                                 { return "MockLock" }

var mockOptions = options.NewTerragruntOptionsForTest("lock_test")

func TestWithLockNoop(t *testing.T) {
	t.Parallel()

	err := WithLock(NoopLock{}, mockOptions, func() error { return nil })
	assert.Nil(t, err)
}

// A mock lock that returns an error on AcquireLock
type ErrorOnAcquireLock struct{}

var ErrorOnAcquire = fmt.Errorf("error-on-acquire")

func (lock ErrorOnAcquireLock) AcquireLock(terragruntOptions *options.TerragruntOptions) error {
	return ErrorOnAcquire
}
func (lock ErrorOnAcquireLock) ReleaseLock(terragruntOptions *options.TerragruntOptions) error {
	return nil
}
func (lock ErrorOnAcquireLock) String() string { return "ErrorOnAcquireLock" }

func TestWithLockErrorOnAcquire(t *testing.T) {
	t.Parallel()

	actionDidExecute := false

	err := WithLock(ErrorOnAcquireLock{}, mockOptions, func() error {
		actionDidExecute = true
		return nil
	})

	assert.Equal(t, ErrorOnAcquire, err, "Expected to get back the error the AcquireLock function returned")
	assert.False(t, actionDidExecute, "Action shouldn't execute when AcquireLock fails!")
}

// A mock lock that returns an error on Release
type ErrorOnReleaseLock struct{}

var ErrorOnRelease = fmt.Errorf("error-on-release")

func (lock ErrorOnReleaseLock) AcquireLock(terragruntOptions *options.TerragruntOptions) error {
	return nil
}
func (lock ErrorOnReleaseLock) ReleaseLock(terragruntOptions *options.TerragruntOptions) error {
	return ErrorOnRelease
}
func (lock ErrorOnReleaseLock) String() string { return "ErrorOnRelease" }

func TestWithLockErrorOnRelease(t *testing.T) {
	t.Parallel()

	actionDidExecute := false

	err := WithLock(ErrorOnReleaseLock{}, mockOptions, func() error {
		actionDidExecute = true
		return nil
	})

	assert.Equal(t, ErrorOnRelease, err, "Expected to get back the error the ReleaseLock function returned")
	assert.True(t, actionDidExecute, "Action didn't execute!")
}

func TestWithLockErrorOnReleaseAndErrorInAction(t *testing.T) {
	t.Parallel()

	actionDidExecute := false
	actionErr := fmt.Errorf("error-in-action")

	err := WithLock(ErrorOnReleaseLock{}, mockOptions, func() error {
		actionDidExecute = true
		return actionErr
	})

	assert.Equal(t, actionErr, err, "Expected to get back the error the action returned")
	assert.True(t, actionDidExecute, "Action didn't execute!")
}

func TestWithLockErrorOnReleaseAndPanicInAction(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		assert.NotNil(t, r, "Expected action to panic, but recover returned nil")
	}()

	actionDidExecute := false
	actionErr := fmt.Errorf("error-in-action")

	err := WithLock(ErrorOnReleaseLock{}, mockOptions, func() error {
		actionDidExecute = true
		panic(actionErr)
	})

	assert.Equal(t, ErrorOnRelease, err, "Expected to get back the error the ReleaseLock function returned, as release should happen even if there is a panic")
	assert.True(t, actionDidExecute, "Action didn't execute!")
}
