package getter_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryGetterMode pins ModeDir for tfr sources, the only mode
// the registry getter supports.
func TestRegistryGetterMode(t *testing.T) {
	t.Parallel()

	r := getter.NewRegistryGetter(logger.CreateLogger())

	mode, err := r.Mode(t.Context(), &url.URL{Scheme: "tfr"})
	require.NoError(t, err)
	assert.Equal(t, getter.ModeDir, mode)
}

// TestRegistryGetterGetFile pins the unsupported error so a future refactor
// doesn't silently flip the behavior.
func TestRegistryGetterGetFile(t *testing.T) {
	t.Parallel()

	r := getter.NewRegistryGetter(logger.CreateLogger())

	err := r.GetFile(t.Context(), &getter.Request{})
	require.Error(t, err)
}

// TestRegistryGetterDetect pins the tfr:// detection logic, including the
// forced-getter shortcut and the rejection of non-tfr schemes.
func TestRegistryGetterDetect(t *testing.T) {
	t.Parallel()

	r := getter.NewRegistryGetter(logger.CreateLogger())

	tests := []struct {
		req  *getter.Request
		name string
		want bool
	}{
		{name: "forced tfr", req: &getter.Request{Forced: "tfr", Src: "anything"}, want: true},
		{name: "tfr scheme", req: &getter.Request{Src: "tfr://example.com/foo/bar/baz?version=1"}, want: true},
		{name: "https scheme", req: &getter.Request{Src: "https://example.com/foo"}, want: false},
		{name: "no scheme", req: &getter.Request{Src: "github.com/foo/bar"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := r.Detect(tt.req)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCASProtocolGetterMode pins ModeDir for cas:: sources.
func TestCASProtocolGetterMode(t *testing.T) {
	t.Parallel()

	g := getter.NewCASProtocolGetter(logger.CreateLogger(), nil)

	mode, err := g.Mode(t.Context(), &url.URL{Scheme: "cas"})
	require.NoError(t, err)
	assert.Equal(t, getter.ModeDir, mode)
}

// TestCASProtocolGetterGetFile pins the unsupported error so callers cannot
// accidentally treat a CAS reference as a single-file download.
func TestCASProtocolGetterGetFile(t *testing.T) {
	t.Parallel()

	g := getter.NewCASProtocolGetter(logger.CreateLogger(), nil)

	err := g.GetFile(t.Context(), &getter.Request{})
	require.ErrorIs(t, err, cas.ErrGetFileNotSupported)
}

// TestGitGetterForcesEnableSymlinks pins the only behavior GitGetter adds on
// top of the upstream git getter: it forces req.DisableSymlinks to false for
// the inner Get call and restores the caller's value when Get returns.
func TestGitGetterForcesEnableSymlinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		callerValue bool
	}{
		{name: "caller_disabled_symlinks", callerValue: true},
		{name: "caller_enabled_symlinks", callerValue: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &recordingGetter{}
			g := getter.NewGitGetter().WithInner(stub)

			req := &getter.Request{DisableSymlinks: tc.callerValue}
			require.NoError(t, g.Get(t.Context(), req))

			assert.False(t, stub.observedDisableSymlinks, "GitGetter must enable symlinks for the inner Get")
			assert.Equal(t, tc.callerValue, req.DisableSymlinks, "GitGetter must restore the caller's flag")
		})
	}
}

// TestGitGetterGetFilePassthrough pins the passthrough so a future refactor
// can't silently change the contract.
func TestGitGetterGetFilePassthrough(t *testing.T) {
	t.Parallel()

	stub := &recordingGetter{}
	g := getter.NewGitGetter().WithInner(stub)

	require.NoError(t, g.GetFile(t.Context(), &getter.Request{}))
	assert.Equal(t, 1, stub.getFileCalls)
}

// recordingGetter is a minimal Getter implementation. Tests use it to
// observe how GitGetter calls into its inner getter without standing up a
// real git remote.
type recordingGetter struct {
	getCalls                int
	getFileCalls            int
	observedDisableSymlinks bool
}

func (g *recordingGetter) Get(_ context.Context, req *getter.Request) error {
	g.getCalls++
	g.observedDisableSymlinks = req.DisableSymlinks

	return nil
}

func (g *recordingGetter) GetFile(_ context.Context, _ *getter.Request) error {
	g.getFileCalls++
	return nil
}

func (g *recordingGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeAny, nil
}

func (g *recordingGetter) Detect(_ *getter.Request) (bool, error) {
	return true, nil
}
