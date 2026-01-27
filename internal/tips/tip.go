// Package tips provides utilities for displaying helpful tips to users during specific workflows.
// Tips are informational messages that can help users troubleshoot issues or learn about features.
//
// Tips can be disabled globally using --no-tips or individually using --no-tip <tip-name>.
package tips

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Tip represents a helpful tip displayed to users.
type Tip struct {
	// Name is a unique identifier for the tip
	Name string
	// Message is the message to display when the tip is triggered
	Message string
	// OnceShow is a sync.Once to ensure the tip is only shown once per session
	OnceShow sync.Once
	// disabled is an atomic boolean to ensure the tip is only disabled once per session
	disabled int32
}

// Tips is a collection of Tip pointers.
type Tips []*Tip

// Evaluate displays the tip if not disabled and not already shown.
func (tip *Tip) Evaluate(l log.Logger) {
	if tip == nil || tip.isDisabled() || l == nil {
		return
	}

	tip.OnceShow.Do(func() {
		l.Info(tip.Message)
	})
}

// Disable disables this tip from being shown.
func (tip *Tip) Disable() {
	atomic.StoreInt32(&tip.disabled, 1)
}

func (tip *Tip) isDisabled() bool {
	return atomic.LoadInt32(&tip.disabled) == 1
}

// Names returns all tip names.
func (t Tips) Names() []string {
	names := make([]string, 0, len(t))

	for _, tip := range t {
		names = append(names, tip.Name)
	}

	slices.Sort(names)

	return names
}

// Find searches and returns the tip by the given `name`.
func (t Tips) Find(name string) *Tip {
	for _, tip := range t {
		if tip.Name == name {
			return tip
		}
	}

	return nil
}

// DisableAll disables all tips such that they aren't shown.
func (t Tips) DisableAll() {
	for _, tip := range t {
		tip.Disable()
	}
}

// DisableTip validates that the specified tip name is valid and disables this tip.
func (t Tips) DisableTip(name string) error {
	found := t.Find(name)
	if found == nil {
		return NewInvalidTipNameError(t.Names())
	}

	found.Disable()

	return nil
}
