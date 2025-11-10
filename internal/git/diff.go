package git

import (
	"bytes"
	"context"
	"strings"
)

func (g *GitRunner) Diff(ctx context.Context, fromRef, toRef string) ([]string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	cmd := g.prepareCommand(ctx, "diff", "--name-only", "--diff-filter=ACDMR", fromRef, toRef)
	cmd.Dir = g.WorkDir

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &WrappedError{
			Op:      "git_diff",
			Context: stderr.String(),
			Err:     ErrCommandSpawn,
		}
	}

	return strings.Split(strings.TrimSpace(stdout.String()), "\n"), nil
}
