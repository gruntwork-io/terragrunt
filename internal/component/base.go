package component

import (
	"fmt"
	"path/filepath"
	"sync"
)

// baseComponent holds fields and methods shared by Unit and Stack.
// Embed this struct to get Path, DisplayPath, External, Reading,
// DiscoveryContext, Origin, dependency/dependent management, and locking.
type baseComponent struct {
	discoveryContext *DiscoveryContext
	path             string
	reading          []string
	dependencies     Components
	dependents       Components
	mu               sync.RWMutex
	external         bool
}

func newBaseComponent(path string) baseComponent {
	return baseComponent{
		path:             path,
		discoveryContext: &DiscoveryContext{},
		dependencies:     make(Components, 0),
		dependents:       make(Components, 0),
	}
}

// Path returns the path to the component.
func (b *baseComponent) Path() string {
	return b.path
}

// DisplayPath returns the path relative to DiscoveryContext.WorkingDir for display purposes.
// Falls back to the original path if relative path calculation fails or WorkingDir is empty.
func (b *baseComponent) DisplayPath() string {
	if b.discoveryContext == nil || b.discoveryContext.WorkingDir == "" {
		return b.path
	}

	if rel, err := filepath.Rel(b.discoveryContext.WorkingDir, b.path); err == nil {
		return rel
	}

	return b.path
}

// External returns whether the component is external.
func (b *baseComponent) External() bool {
	return b.external
}

// SetExternal marks the component as external.
func (b *baseComponent) SetExternal() {
	b.external = true
}

// Reading returns the list of files being read by this component.
func (b *baseComponent) Reading() []string {
	return b.reading
}

// SetReading sets the list of files being read by this component.
func (b *baseComponent) SetReading(files ...string) {
	b.reading = files
}

// DiscoveryContext returns the discovery context for this component.
func (b *baseComponent) DiscoveryContext() *DiscoveryContext {
	return b.discoveryContext
}

// SetDiscoveryContext sets the discovery context for this component.
func (b *baseComponent) SetDiscoveryContext(ctx *DiscoveryContext) {
	b.discoveryContext = ctx
}

// Origin returns the origin of the discovery context for this component.
func (b *baseComponent) Origin() Origin {
	if b.discoveryContext == nil {
		return OriginUnknown
	}

	return b.discoveryContext.Origin()
}

// Dependencies returns the dependencies of the component.
func (b *baseComponent) Dependencies() Components {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.dependencies
}

// Dependents returns the dependents of the component.
func (b *baseComponent) Dependents() Components {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.dependents
}

// addDependency adds dep as a dependency of self and self as a dependent of dep.
// Uses consistent lock ordering (lower Path first) to prevent ABBA deadlock.
func (b *baseComponent) addDependency(self Component, dep Component) {
	if self.Path() == dep.Path() {
		return
	}

	first, second := orderByPath(self, dep)

	first.mu.Lock()
	defer first.mu.Unlock()

	second.mu.Lock()
	defer second.mu.Unlock()

	b.ensureDependencyLocked(dep)
	extractBase(dep).ensureDependentLocked(self)
}

// addDependent adds dep as a dependent of self and self as a dependency of dep.
// Uses consistent lock ordering (lower Path first) to prevent ABBA deadlock.
func (b *baseComponent) addDependent(self Component, dep Component) {
	if self.Path() == dep.Path() {
		return
	}

	first, second := orderByPath(self, dep)

	first.mu.Lock()
	defer first.mu.Unlock()

	second.mu.Lock()
	defer second.mu.Unlock()

	b.ensureDependentLocked(dep)
	extractBase(dep).ensureDependencyLocked(self)
}

// ensureDependencyLocked adds dep if not already present.
// Caller must hold b.mu write lock.
func (b *baseComponent) ensureDependencyLocked(dep Component) {
	for _, d := range b.dependencies {
		if d.Path() == dep.Path() {
			return
		}
	}

	b.dependencies = append(b.dependencies, dep)
}

// ensureDependentLocked adds dep if not already present.
// Caller must hold b.mu write lock.
func (b *baseComponent) ensureDependentLocked(dep Component) {
	for _, d := range b.dependents {
		if d.Path() == dep.Path() {
			return
		}
	}

	b.dependents = append(b.dependents, dep)
}

// extractBase returns the embedded baseComponent from a Component.
// Only Unit and Stack implement Component, and both embed baseComponent.
func extractBase(c Component) *baseComponent {
	switch v := c.(type) {
	case *Unit:
		return &v.baseComponent
	case *Stack:
		return &v.baseComponent
	default:
		panic(fmt.Sprintf("unknown Component type: %T", c))
	}
}

// orderByPath returns two baseComponents ordered by Path() for consistent lock ordering.
func orderByPath(a, b Component) (*baseComponent, *baseComponent) {
	ba, bb := extractBase(a), extractBase(b)
	if a.Path() <= b.Path() {
		return ba, bb
	}

	return bb, ba
}
