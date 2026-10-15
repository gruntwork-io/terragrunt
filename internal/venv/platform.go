package venv

import (
	"fmt"
	"os"
)

// Platform carries the operating-system handles used below the CLI boundary.
type Platform struct {
	UserHomeDir    func() (string, error)
	UserCacheDir   func() (string, error)
	UserConfigDir  func() (string, error)
	TempDir        func() string
	Getwd          func() (string, error)
	GetPID         func() int
	ReplaceEnviron func(env map[string]string) error
	GOOS           string
	GOARCH         string
}

// WithGOOS returns a copy of v whose operating-system identifier is goos.
func (v *Venv) WithGOOS(goos string) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.GOOS = goos

	c := *v
	c.Platform = &platform

	return &c
}

// WithGOARCH returns a copy of v whose architecture identifier is goarch.
func (v *Venv) WithGOARCH(goarch string) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.GOARCH = goarch

	c := *v
	c.Platform = &platform

	return &c
}

// WithUserHomeDir returns a copy of v whose home-directory lookup is userHomeDir.
func (v *Venv) WithUserHomeDir(userHomeDir func() (string, error)) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.UserHomeDir = userHomeDir

	c := *v
	c.Platform = &platform

	return &c
}

// WithUserCacheDir returns a copy of v whose cache-directory lookup is userCacheDir.
func (v *Venv) WithUserCacheDir(userCacheDir func() (string, error)) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.UserCacheDir = userCacheDir

	c := *v
	c.Platform = &platform

	return &c
}

// WithUserConfigDir returns a copy of v whose config-directory lookup is userConfigDir.
func (v *Venv) WithUserConfigDir(userConfigDir func() (string, error)) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.UserConfigDir = userConfigDir

	c := *v
	c.Platform = &platform

	return &c
}

// WithTempDir returns a copy of v whose temp-directory lookup is tempDir.
func (v *Venv) WithTempDir(tempDir func() string) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.TempDir = tempDir

	c := *v
	c.Platform = &platform

	return &c
}

// WithReplaceEnviron returns a copy of v whose process environment
// replacement is replaceEnviron.
func (v *Venv) WithReplaceEnviron(replaceEnviron func(env map[string]string) error) *Venv {
	v.RequirePlatform()

	platform := *v.Platform
	platform.ReplaceEnviron = replaceEnviron

	c := *v
	c.Platform = &platform

	return &c
}

// RequirePlatform panics with [ErrVenvPlatformUnset] when Platform is nil.
// The platform builders call this so that refining one handle on a Venv that
// carries no platform at all fails at the builder rather than silently
// producing a platform whose other handles are nil.
func (v *Venv) RequirePlatform() {
	if v.Platform == nil {
		panic(ErrVenvPlatformUnset)
	}
}

// RequireGOOS panics with [ErrVenvGOOSUnset] when GOOS is empty.
func (v *Venv) RequireGOOS() {
	if v.Platform == nil || v.Platform.GOOS == "" {
		panic(ErrVenvGOOSUnset)
	}
}

// RequireGOARCH panics with [ErrVenvGOARCHUnset] when GOARCH is empty. The two
// halves of a platform identifier have to come from the same place: a run that
// took its GOOS from the venv and its architecture from the binary would name
// a target that exists on no machine.
func (v *Venv) RequireGOARCH() {
	if v.Platform == nil || v.Platform.GOARCH == "" {
		panic(ErrVenvGOARCHUnset)
	}
}

// RequireUserHomeDir panics with [ErrVenvUserHomeDirUnset] when UserHomeDir is nil.
func (v *Venv) RequireUserHomeDir() {
	if v.Platform == nil || v.Platform.UserHomeDir == nil {
		panic(ErrVenvUserHomeDirUnset)
	}
}

// RequireUserCacheDir panics with [ErrVenvUserCacheDirUnset] when UserCacheDir is nil.
func (v *Venv) RequireUserCacheDir() {
	if v.Platform == nil || v.Platform.UserCacheDir == nil {
		panic(ErrVenvUserCacheDirUnset)
	}
}

// RequireUserConfigDir panics with [ErrVenvUserConfigDirUnset] when UserConfigDir is nil.
func (v *Venv) RequireUserConfigDir() {
	if v.Platform == nil || v.Platform.UserConfigDir == nil {
		panic(ErrVenvUserConfigDirUnset)
	}
}

// RequireReplaceEnviron panics with [ErrVenvReplaceEnvironUnset] when
// ReplaceEnviron is nil.
func (v *Venv) RequireReplaceEnviron() {
	if v.Platform == nil || v.Platform.ReplaceEnviron == nil {
		panic(ErrVenvReplaceEnvironUnset)
	}
}

// RequireTempDir panics with [ErrVenvTempDirUnset] when TempDir is nil.
func (v *Venv) RequireTempDir() {
	if v.Platform == nil || v.Platform.TempDir == nil {
		panic(ErrVenvTempDirUnset)
	}
}

// replaceOSEnviron clears the process environment, then sets every variable
// in env.
func replaceOSEnviron(env map[string]string) error {
	os.Clearenv()

	for name, value := range env {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("setting %s in the process environment: %w", name, err)
		}
	}

	return nil
}
