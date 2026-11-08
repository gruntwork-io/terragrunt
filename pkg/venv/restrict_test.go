package venv_test

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/venv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// denials collects what a restricted venv refused. Restrict may report from
// several goroutines, so it locks.
type denials struct {
	got []venv.Denial
	mu  sync.Mutex
}

func (d *denials) observe(dn venv.Denial) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.got = append(d.got, dn)
}

func (d *denials) kinds() []venv.DenialKind {
	d.mu.Lock()
	defer d.mu.Unlock()

	kinds := make([]venv.DenialKind, 0, len(d.got))
	for _, dn := range d.got {
		kinds = append(kinds, dn.Kind)
	}

	return kinds
}

// ranVenv returns an in-memory venv whose executor answers every command it
// is handed with "ran", so a test can tell a command that reached it from one
// a restricted venv answered itself.
func ranVenv() *venv.Venv {
	return venv.MemVenv().WithHandler(func(context.Context, vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("ran")}
	})
}

// TestRestrictStubsAConfigCommandWithoutExec pins that a run_cmd program gets
// empty output and no error, since a run_cmd error aborts the whole parse.
func TestRestrictStubsAConfigCommandWithoutExec(t *testing.T) {
	t.Parallel()

	var d denials

	v := venv.Restrict(ranVenv(), venv.Policy{Observe: d.observe})

	out, err := v.Exec.Command(t.Context(), "jq", "-r", ".name").Output()
	require.NoError(t, err)
	assert.Empty(t, out)
	assert.Equal(t, []venv.DenialKind{venv.StubbedCommand}, d.kinds())
	assert.Equal(t, "jq", d.got[0].Program)
	assert.Equal(t, "jq -r .name", d.got[0].Cmdline)
}

// TestRestrictDeniesIaCWithoutExec pins that tofu fails rather than stubbing,
// so a dependency fetch falls back to mock_outputs instead of reading empty
// outputs as real ones.
func TestRestrictDeniesIaCWithoutExec(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"tofu", "terraform", "/opt/bin/tofu.exe", "/opt/custom-iac"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var d denials

			v := venv.Restrict(ranVenv(), venv.Policy{TFPath: "/opt/custom-iac", Observe: d.observe})

			err := v.Exec.Command(t.Context(), name, "output", "-json").Run()
			require.ErrorIs(t, err, venv.ErrExecDenied)
			assert.Equal(t, []venv.DenialKind{venv.DeniedIaC}, d.kinds())
		})
	}
}

// TestRestrictDeniesATrustedCommandWithoutExec pins that a command Terragrunt
// starts on its own account fails, so a source it would download is reported
// as missing rather than read as empty.
func TestRestrictDeniesATrustedCommandWithoutExec(t *testing.T) {
	t.Parallel()

	var d denials

	v := venv.Restrict(ranVenv(), venv.Policy{Observe: d.observe})

	ctx := vexec.WithTrustedCommand(t.Context())
	err := v.Exec.Command(ctx, "git", "clone", "https://example.invalid/repo.git").Run()
	require.ErrorIs(t, err, venv.ErrExecDenied)
	assert.Equal(t, []venv.DenialKind{venv.DeniedTrustedCommand}, d.kinds())
}

// TestRestrictStubsAConfigGitWithoutExec pins that a git a configuration
// names gets the same empty answer as any other program. get_repo_root()
// walks the filesystem rather than running git, so nothing needs a stand-in
// answer, and a directory would be a wrong one for `git rev-parse HEAD`.
func TestRestrictStubsAConfigGitWithoutExec(t *testing.T) {
	t.Parallel()

	var d denials

	v := venv.Restrict(ranVenv(), venv.Policy{Observe: d.observe})

	out, err := v.Exec.Command(t.Context(), "git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	assert.Empty(t, out)
	assert.Equal(t, []venv.DenialKind{venv.StubbedCommand}, d.kinds())
}

// TestRestrictRedactsCredentialsInCmdline pins that a credential a
// configuration spells into a URL argument never reaches an observer.
func TestRestrictRedactsCredentialsInCmdline(t *testing.T) {
	t.Parallel()

	var d denials

	v := venv.Restrict(ranVenv(), venv.Policy{Observe: d.observe})

	require.NoError(t, v.Exec.Command(t.Context(), "curl", "https://user:hunter2@example.invalid/x").Run())
	require.Len(t, d.got, 1)
	assert.NotContains(t, d.got[0].Cmdline, "hunter2")
}

// TestRestrictRefusesListeningWithoutExec pins that no socket opens, since the
// only server Terragrunt starts is the provider cache a tofu process fetches
// from.
func TestRestrictRefusesListeningWithoutExec(t *testing.T) {
	t.Parallel()

	v := venv.Restrict(venv.MemVenv(), venv.Policy{})

	_, err := v.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.ErrorIs(t, err, venv.ErrListenDenied)
}

// TestRestrictRunsTrustedCommandsWithExec pins that granting exec lets
// Terragrunt's own commands reach the real executor without a pattern.
func TestRestrictRunsTrustedCommandsWithExec(t *testing.T) {
	t.Parallel()

	v := venv.Restrict(ranVenv(), venv.Policy{Granted: venv.Capabilities{venv.CapabilityExec: {}}})

	out, err := v.Exec.Command(vexec.WithTrustedCommand(t.Context()), "tofu", "init").Output()
	require.NoError(t, err)
	assert.Equal(t, "ran", string(out))
}

// TestRestrictRunsAConfigCommandAPatternAllows pins that an allow-cmd pattern
// is what lets a program a configuration names run.
func TestRestrictRunsAConfigCommandAPatternAllows(t *testing.T) {
	t.Parallel()

	jq, err := venv.ParseAllowCmd("jq -r *")
	require.NoError(t, err)

	var d denials

	v := venv.Restrict(ranVenv(), venv.Policy{
		Granted:   venv.Capabilities{venv.CapabilityExec: {}},
		AllowCmds: []venv.AllowCmd{jq},
		Observe:   d.observe,
	})

	out, err := v.Exec.Command(t.Context(), "jq", "-r", ".name").Output()
	require.NoError(t, err)
	assert.Equal(t, "ran", string(out))

	out, err = v.Exec.Command(t.Context(), "jq", ".name").Output()
	require.NoError(t, err)
	assert.Empty(t, out, "arguments the pattern does not match are refused")
	assert.Equal(t, []venv.DenialKind{venv.RefusedCommand}, d.kinds())
}

// TestRestrictRunsAnAllowedProgramWithAnyArguments pins [venv.AllowProgram],
// which a caller uses for a program a person approved.
func TestRestrictRunsAnAllowedProgramWithAnyArguments(t *testing.T) {
	t.Parallel()

	v := venv.Restrict(ranVenv(), venv.Policy{
		Granted:   venv.Capabilities{venv.CapabilityExec: {}},
		AllowCmds: []venv.AllowCmd{venv.AllowProgram("aws")},
	})

	out, err := v.Exec.Command(t.Context(), "aws", "sts", "get-caller-identity").Output()
	require.NoError(t, err)
	assert.Equal(t, "ran", string(out))
}

// TestRestrictRefusesAShippedBinaryNamedLikeAnAllowedOne pins that a pattern
// for git does not cover a ./git the configuration's tree carries.
func TestRestrictRefusesAShippedBinaryNamedLikeAnAllowedOne(t *testing.T) {
	t.Parallel()

	v := venv.Restrict(ranVenv(), venv.Policy{
		Granted:   venv.Capabilities{venv.CapabilityExec: {}},
		AllowCmds: []venv.AllowCmd{venv.AllowProgram("git")},
	})

	out, err := v.Exec.Command(t.Context(), "./git", "status").Output()
	require.NoError(t, err)
	assert.Empty(t, out)
}

// TestRestrictFailsARefusedCommandUnderRefuseWithError pins the mode a run
// that changes infrastructure uses, where a stub would read as success.
func TestRestrictFailsARefusedCommandUnderRefuseWithError(t *testing.T) {
	t.Parallel()

	v := venv.Restrict(ranVenv(), venv.Policy{
		Granted: venv.Capabilities{venv.CapabilityExec: {}},
		Refusal: venv.RefuseWithError,
	})

	err := v.Exec.Command(t.Context(), "./check.sh").Run()
	require.ErrorIs(t, err, venv.ErrCmdNotAllowed)
}

// TestRestrictDeniesSopsWithoutTheCapability pins that a decrypt fails rather
// than returning empty cleartext, which a configuration would use silently.
func TestRestrictDeniesSopsWithoutTheCapability(t *testing.T) {
	t.Parallel()

	var d denials

	v := venv.Restrict(venv.MemVenv(), venv.Policy{Observe: d.observe})

	_, err := v.Sops.DecryptFile(v.Env, "secrets.enc.json", "json")
	require.ErrorIs(t, err, venv.ErrSopsDenied)
	assert.Equal(t, []venv.DenialKind{venv.DeniedSops}, d.kinds())
	assert.Equal(t, "secrets.enc.json", d.got[0].Path)
}

// TestRestrictDeniesHTTPWithoutTheCapability pins that Terragrunt's own
// requests fail without the http capability.
func TestRestrictDeniesHTTPWithoutTheCapability(t *testing.T) {
	t.Parallel()

	base := venv.MemVenv().WithHTTP(vhttp.NewMemClient(func(context.Context, *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	}))

	v := venv.Restrict(base, venv.Policy{})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.invalid", nil)
	require.NoError(t, err)

	resp, err := v.HTTP.Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}

	require.ErrorIs(t, err, vhttp.ErrNoNetwork)
}

// TestRestrictEmptiesTheEnvironmentWithoutTheCapability pins that get_env()
// reads nothing from the venv a caller narrowed.
func TestRestrictEmptiesTheEnvironmentWithoutTheCapability(t *testing.T) {
	t.Parallel()

	base := venv.MemVenv().WithEnv(map[string]string{"AWS_SECRET_ACCESS_KEY": "hunter2"})

	assert.Empty(t, venv.Restrict(base, venv.Policy{}).Env)
	assert.Equal(t, base.Env, venv.Restrict(base, venv.Policy{
		Granted: venv.Capabilities{venv.CapabilityEnv: {}},
	}).Env)
}

// TestRestrictNeverReadsStdinOrOpensABrowser pins the console handles no
// capability grants.
func TestRestrictNeverReadsStdinOrOpensABrowser(t *testing.T) {
	t.Parallel()

	v := venv.Restrict(venv.OSVenv(), venv.Policy{Granted: venv.Capabilities{venv.CapabilityExec: {}}})

	in, err := io.ReadAll(v.Stdin)
	require.NoError(t, err)
	assert.Empty(t, in)
	require.ErrorIs(t, v.Browser.Open(t.Context(), "https://example.invalid"), vbrowser.ErrNoBrowser)
}

// TestPolicyValidateRefusesAllowCmdsWithoutExec pins that a pattern granted
// without exec is reported, since Restrict would never consult it.
func TestPolicyValidateRefusesAllowCmdsWithoutExec(t *testing.T) {
	t.Parallel()

	p := venv.Policy{AllowCmds: []venv.AllowCmd{venv.AllowProgram("jq")}}
	require.ErrorIs(t, p.Validate(), venv.ErrAllowCmdRequiresExec)

	p.Granted = venv.Capabilities{venv.CapabilityExec: {}}
	require.NoError(t, p.Validate())
}
