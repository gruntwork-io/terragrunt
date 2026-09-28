package config_test

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

const (
	isolationAuthCmd = "auth-cmd"
	isolationEchoCmd = "echoer"
)

func TestRunCommandTraceParentLeavesCallerEnvWithRacing(t *testing.T) {
	t.Parallel()

	recorder := &isolationExecRecorder{}
	v := venvtest.New().
		WithEnv(map[string]string{"CALLER": "value"}).
		WithExec(recorder.exec())
	want := maps.Clone(v.Env)

	ctx, pctx := newTestParsingContext(t, v, venvtest.Root("/repo/unit/terragrunt.hcl"))
	ctx = contextWithTraceParent(config.WithConfigValues(ctx))
	l := logger.CreateLogger()

	const calls = 8

	group, groupCtx := errgroup.WithContext(ctx)

	for i := range calls {
		group.Go(func() error {
			_, err := config.RunCommand(
				groupCtx,
				pctx.Clone(),
				l,
				[]string{"--terragrunt-no-cache", isolationEchoCmd, strconv.Itoa(i)},
			)

			return err
		})
	}

	require.NoError(t, group.Wait())

	assert.Equal(t, want, v.Env)
	assert.Equal(t, calls, recorder.countWithEnv(telemetry.TraceParentEnv))
}

func TestDependencyOutputAuthAndExtraArgsEnvLeaveCallerEnv(t *testing.T) {
	t.Parallel()

	recorder := &isolationExecRecorder{}
	ctx, pctx, v, configPath := prepareIsolationFixture(t, recorder, 1)
	want := maps.Clone(v.Env)

	cfg, err := config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), configPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "from-native-output", cfg.Inputs["result0"])

	assert.Equal(t, want, v.Env)

	outputs := recorder.outputEnvs()
	require.NotEmpty(t, outputs)

	for _, env := range outputs {
		assert.Contains(t, env, "AUTH_TOKEN=from-auth")
		assert.Contains(t, env, "OUTPUT_ONLY=1")
	}
}

func TestDependencyOutputsWithRunCmdLeaveCallerEnvWithRacing(t *testing.T) {
	t.Parallel()

	const producers = 4

	recorder := &isolationExecRecorder{}
	ctx, pctx, v, configPath := prepareIsolationFixture(t, recorder, producers)
	ctx = contextWithTraceParent(ctx)
	want := maps.Clone(v.Env)

	cfg, err := config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), configPath, nil)
	require.NoError(t, err)

	for i := range producers {
		assert.Equal(t, "from-native-output", cfg.Inputs["result"+strconv.Itoa(i)])
	}

	assert.Equal(t, want, v.Env)
	assert.Positive(t, recorder.countWithEnv(telemetry.TraceParentEnv))
}

type isolationExecRecorder struct {
	envs    [][]string
	outputs [][]string
	mu      sync.Mutex
}

func (r *isolationExecRecorder) exec() vexec.Exec {
	return vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		r.mu.Lock()
		r.envs = append(r.envs, slices.Clone(inv.Env))

		if slices.Contains(inv.Args, "output") {
			r.outputs = append(r.outputs, slices.Clone(inv.Env))
		}
		r.mu.Unlock()

		switch {
		case inv.Name == isolationAuthCmd:
			return vexec.Result{Stdout: []byte(`{"envs":{"AUTH_TOKEN":"from-auth"}}`)}
		case inv.Name == isolationEchoCmd:
			return vexec.Result{Stdout: []byte(strings.Join(inv.Args, " ") + "\n")}
		case slices.Contains(inv.Args, "output"):
			return vexec.Result{Stdout: terraformOutput("from-native-output")}
		}

		return vexec.Result{}
	})
}

func (r *isolationExecRecorder) countWithEnv(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0

	for _, env := range r.envs {
		if slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, key+"=") }) {
			count++
		}
	}

	return count
}

func (r *isolationExecRecorder) outputEnvs() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.outputs)
}

// prepareIsolationFixture writes a consumer with one dependency per producer.
// Each producer calls run_cmd, sets output env vars through extra_arguments,
// and uses an S3 backend the direct state reader rejects, so outputs come from
// a native output call after credentials are obtained.
func prepareIsolationFixture(
	t *testing.T,
	recorder *isolationExecRecorder,
	producers int,
) (context.Context, *config.ParsingContext, *venv.Venv, string) {
	t.Helper()

	consumerPath := venvtest.Root("/repo/consumer/terragrunt.hcl")

	v := venvtest.New().
		WithEnv(map[string]string{"CALLER": "value"}).
		WithExec(recorder.exec())

	require.NoError(t, v.FS.MkdirAll(filepath.Dir(consumerPath), 0o700))

	var (
		dependencies strings.Builder
		inputs       strings.Builder
	)

	for i := range producers {
		name := "producer" + strconv.Itoa(i)
		producerPath := venvtest.Root("/repo/" + name + "/terragrunt.hcl")

		require.NoError(t, v.FS.MkdirAll(filepath.Dir(producerPath), 0o700))

		producer := fmt.Sprintf(`locals {
  tag = run_cmd(%q, %q)
}

remote_state {
  backend = "s3"
  config = {
    bucket               = "state-bucket"
    key                  = "service.tfstate"
    region               = "us-east-1"
    workspace_key_prefix = "/workspaces"
  }
}

terraform {
  extra_arguments "output_env" {
    commands = ["output"]
    env_vars = {
      OUTPUT_ONLY = "1"
    }
  }
}
`, isolationEchoCmd, name)
		require.NoError(t, vfs.WriteFile(v.FS, producerPath, []byte(producer), 0o600))

		fmt.Fprintf(&dependencies, "dependency %q {\n  config_path = \"../%s\"\n}\n\n", name, name)
		fmt.Fprintf(&inputs, "  result%d = dependency.%s.outputs.producer_value\n", i, name)
	}

	consumer := fmt.Sprintf(`locals {
  tag = run_cmd(%q, "consumer")
}

%sinputs = {
%s}
`, isolationEchoCmd, dependencies.String(), inputs.String())
	require.NoError(t, vfs.WriteFile(v.FS, consumerPath, []byte(consumer), 0o600))

	ctx, pctx := newTestParsingContext(t, v, consumerPath)
	ctx = config.WithConfigValues(ctx)
	pctx.OriginalTerragruntConfigPath = consumerPath
	pctx.AuthProviderCmd = isolationAuthCmd

	return ctx, pctx, v, consumerPath
}

func contextWithTraceParent(ctx context.Context) context.Context {
	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
	}))
}
