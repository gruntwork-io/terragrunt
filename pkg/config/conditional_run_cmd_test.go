package config_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHCLConditionalRunCmd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		expr  string
		want  string
		calls []string
	}{
		{
			name: "true",
			expr: `local.condition ? run_cmd("A") : run_cmd("B")`,
			want: "A", calls: []string{"A"},
		},
		{
			name: "false",
			expr: `!local.condition ? run_cmd("A") : run_cmd("B")`,
			want: "B", calls: []string{"B"},
		},
		{
			name: "nested",
			expr: `local.condition ? (false ? run_cmd("A") : run_cmd("B")) : run_cmd("C")`,
			want: "B", calls: []string{"B"},
		},
		{
			name: "function argument",
			expr: `upper(local.condition ? run_cmd("A") : run_cmd("B"))`,
			want: "A", calls: []string{"A"},
		},
		{
			name: "template",
			expr: `"value: ${local.condition ? run_cmd("A") : run_cmd("B")}"`,
			want: "value: A", calls: []string{"A"},
		},
		{
			name: "template interpolation",
			expr: `"${local.condition ? run_cmd("A") : run_cmd("B")}"`,
			want: "A", calls: []string{"A"},
		},
		{
			name: "template loop",
			expr: `"%{ for flag in [true, false] }${flag ? run_cmd("A") : run_cmd("B")}%{ endfor }"`,
			want: "AB", calls: []string{"A", "B"},
		},
		{
			name: "object",
			expr: `({ result = local.condition ? run_cmd("A") : run_cmd("B") }).result`,
			want: "A", calls: []string{"A"},
		},
		{
			name: "tuple",
			expr: `[local.condition ? run_cmd("A") : run_cmd("B")][0]`,
			want: "A", calls: []string{"A"},
		},
		{
			name: "for expression",
			expr: `join(",", [for flag in [true, false] : flag ? run_cmd("A") : run_cmd("B")])`,
			want: "A,B", calls: []string{"A", "B"},
		},
		{
			name: "splat",
			expr: `join(",", [{ A = "selected" }][*][true ? run_cmd("A") : run_cmd("B")])`,
			want: "selected", calls: []string{"A"},
		},
		{
			name: "nested command arguments",
			expr: `true ? run_cmd("A") : run_cmd(run_cmd("B"))`,
			want: "A", calls: []string{"A"},
		},
		{
			name: "unselected nested condition",
			expr: `false ? (run_cmd("predicate") == "true" ? run_cmd("B") : run_cmd("C")) : run_cmd("A")`,
			want: "A", calls: []string{"A"},
		},
		{
			name: "binary operation",
			expr: `tostring((true ? run_cmd("A") : run_cmd("B")) == "A")`,
			want: "true", calls: []string{"A"},
		},
		{
			name: "unary operation",
			expr: `tostring(!(true ? run_cmd("A") == "A" : run_cmd("B") == "B"))`,
			want: "false", calls: []string{"A"},
		},
		{
			name: "unselected failure",
			expr: `true ? run_cmd("A") : run_cmd("fail")`,
			want: "A", calls: []string{"A"},
		},
		{
			name: "number conversion",
			expr: `false ? run_cmd("A") : 42`,
			want: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu    sync.Mutex
				calls []string
			)

			exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				mu.Lock()

				calls = append(calls, inv.Name)
				mu.Unlock()

				if inv.Name == "fail" {
					return vexec.Result{ExitCode: 1, Stderr: []byte("command failed\n")}
				}

				return vexec.Result{Stdout: []byte(inv.Name + "\n")}
			})

			v := venvtest.New().WithExec(exec)
			ctx, pctx := newTestParsingContext(t, filepath.Join(t.TempDir(), "terragrunt.hcl"))
			ctx = config.WithCaches(ctx)
			text := fmt.Sprintf("locals {\n  condition = true\n  value = %s\n}\n", tt.expr)

			out, err := config.ParseConfigString(ctx, logger.CreateLogger(), v, pctx, "test.hcl", text, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, out.Locals["value"])
			assert.Equal(t, tt.calls, calls)
		})
	}
}

func TestEvaluateLocalsConditionalRunCmdConditionOnce(t *testing.T) {
	t.Parallel()

	var calls []string

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls = append(calls, inv.Name)

		if inv.Name == "predicate" {
			return vexec.Result{Stdout: []byte("true\n")}
		}

		return vexec.Result{Stdout: []byte(inv.Name + "\n")}
	})
	file, err := hclparse.NewParser().ParseFromString(`locals {
  value = run_cmd("--terragrunt-no-cache", "predicate") == "true" ? run_cmd("A") : run_cmd("B")
}`, "test.hcl")
	require.NoError(t, err)

	v := venvtest.New().WithExec(exec)
	ctx, pctx := newTestParsingContext(t, filepath.Join(t.TempDir(), "terragrunt.hcl"))
	values, err := config.EvaluateLocalsBlock(config.WithCaches(ctx), logger.CreateLogger(), v, pctx, file)
	require.NoError(t, err)
	assert.Equal(t, "A", values["value"].AsString())
	assert.Equal(t, []string{"predicate", "A"}, calls)
}
