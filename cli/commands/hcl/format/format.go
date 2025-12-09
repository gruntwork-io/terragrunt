// Package format recursively looks for hcl files in the directory tree starting at workingDir, and formats them
// based on the language style guides provided by Hashicorp. This is done using the official hcl2 library.
package format

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

var excludePaths = []string{
	util.TerragruntCacheDir,
	util.DefaultBoilerplateDir,
	config.StackDir,
}

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	workingDir := opts.WorkingDir
	targetFile := opts.HclFile
	stdIn := opts.HclFromStdin

	if stdIn {
		if targetFile != "" {
			return errors.Errorf("both stdin and path flags are specified")
		}

		return formatFromStdin(l, opts)
	}

	if targetFile != "" {
		if !filepath.IsAbs(targetFile) {
			targetFile = util.JoinPath(workingDir, targetFile)
		}

		l.Debugf("Formatting hcl file at: %s.", targetFile)

		return formatTgHCL(ctx, l, opts, targetFile)
	}

	var (
		filters filter.Filters
		err     error
	)

	if opts.Experiments.Evaluate(experiment.FilterFlag) {
		filters, err = filter.ParseFilterQueries(opts.FilterQueries)
		if err != nil {
			return errors.New(err)
		}
	}

	// We use lightweight discovery here instead of the full discovery used by
	// the discovery package because we want to find non-comps like includes.
	files := []string{}

	err = filepath.WalkDir(workingDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		basename := filepath.Base(path)
		if slices.Contains(excludePaths, basename) {
			l.Debugf("%s directory ignored by default", path)
			return filepath.SkipDir
		}

		if slices.Contains(opts.HclExclude, basename) {
			l.Debugf("%s directory ignored due to the %s flag", path, ExcludeDirFlagName)
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".hcl") {
			return nil
		}

		files = append(files, path)

		return nil
	})
	if err != nil {
		return errors.New(err)
	}

	var components component.Components

	if opts.Experiments.Evaluate(experiment.FilterFlag) {
		components, err = filters.EvaluateOnFiles(l, files, workingDir)
		if err != nil {
			return errors.New(err)
		}
	} else {
		components = make(component.Components, 0, len(files))
		for _, file := range files {
			components = append(components, component.NewUnit(file))
		}
	}

	g, gctx := errgroup.WithContext(ctx)

	limit := opts.Parallelism
	if limit == options.DefaultParallelism {
		limit = runtime.NumCPU()
	}

	g.SetLimit(limit)

	// Pre-allocate the errs slice with max possible length
	// so we don't need to hold a lock to append to it.
	errs := make([]error, len(components))

	for i, c := range components {
		g.Go(func() error {
			err := formatTgHCL(gctx, l, opts, c.Path())
			if err != nil {
				errs[i] = err
			}

			return nil
		})
	}

	_ = g.Wait()

	return errors.Join(errs...)
}

func formatFromStdin(l log.Logger, opts *options.TerragruntOptions) error {
	contents, err := io.ReadAll(os.Stdin)
	if err != nil {
		l.Errorf("Error reading from stdin: %s", err)

		return fmt.Errorf("error reading from stdin: %w", err)
	}

	if err = checkErrors(l, l.Formatter().DisabledColors(), contents, "stdin"); err != nil {
		l.Errorf("Error parsing hcl from stdin")

		return fmt.Errorf("error parsing hcl from stdin: %w", err)
	}

	newContents := hclwrite.Format(contents)

	buf := bufio.NewWriter(opts.Writer)

	if _, err = buf.Write(newContents); err != nil {
		l.Errorf("Failed to write to stdout")

		return fmt.Errorf("failed to write to stdout: %w", err)
	}

	if err = buf.Flush(); err != nil {
		l.Errorf("Failed to flush to stdout")

		return fmt.Errorf("failed to flush to stdout: %w", err)
	}

	return nil
}

// formatTgHCL uses the hcl2 library to format the hcl file. This will attempt to parse the HCL file first to
// ensure that there are no syntax errors, before attempting to format it.
func formatTgHCL(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, tgHclFile string) error {
	l.Debugf("Formatting %s", tgHclFile)

	info, err := os.Stat(tgHclFile)
	if err != nil {
		l.Errorf("Error retrieving file info of %s", tgHclFile)
		return errors.Errorf("failed to get file info for %s: %w", tgHclFile, err)
	}

	contents, err := os.ReadFile(tgHclFile)
	if err != nil {
		l.Errorf("Error reading %s", tgHclFile)
		return errors.Errorf("failed to read %s: %w", tgHclFile, err)
	}

	err = checkErrors(l, l.Formatter().DisabledColors(), contents, tgHclFile)
	if err != nil {
		l.Errorf("Error parsing %s", tgHclFile)
		return err
	}

	newContents := hclwrite.Format(contents)

	fileUpdated := !bytes.Equal(newContents, contents)

	if opts.Diff && fileUpdated {
		diff, err := bytesDiff(ctx, l, contents, newContents, tgHclFile)
		if err != nil {
			l.Errorf("Failed to generate diff for %s", tgHclFile)
			return err
		}

		_, err = fmt.Fprintf(opts.Writer, "%s\n", diff)
		if err != nil {
			l.Errorf("Failed to print diff for %s", tgHclFile)
			return err
		}
	}

	if opts.Check && fileUpdated {
		return &FileNeedsFormattingError{Path: tgHclFile}
	}

	if fileUpdated {
		l.Infof("%s was updated", tgHclFile)
		return os.WriteFile(tgHclFile, newContents, info.Mode())
	}

	return nil
}

// checkErrors takes in the contents of a hcl file and looks for syntax errors.
func checkErrors(l log.Logger, disableColor bool, contents []byte, tgHclFile string) error {
	parser := hclparse.NewParser()
	_, diags := parser.ParseHCL(contents, tgHclFile)

	writer := writer.New(writer.WithLogger(l), writer.WithDefaultLevel(log.ErrorLevel))
	diagWriter := parser.GetDiagnosticsWriter(writer, disableColor)

	err := diagWriter.WriteDiagnostics(diags)
	if err != nil {
		return errors.New(err)
	}

	if diags.HasErrors() {
		return diags
	}

	return nil
}

// bytesDiff uses GNU diff to display the differences between the contents of HCL file before and after formatting
func bytesDiff(ctx context.Context, l log.Logger, b1, b2 []byte, path string) ([]byte, error) {
	f1, err := os.CreateTemp("", "")
	if err != nil {
		return nil, err
	}

	defer func() {
		if err = f1.Close(); err != nil {
			l.Warnf("Failed to close file %s %v", f1.Name(), err)
		}

		if err = os.Remove(f1.Name()); err != nil {
			l.Warnf("Failed to remove file %s %v", f1.Name(), err)
		}
	}()

	f2, err := os.CreateTemp("", "")
	if err != nil {
		return nil, err
	}

	defer func() {
		if err = f2.Close(); err != nil {
			l.Warnf("Failed to close file %s %v", f2.Name(), err)
		}

		if err = os.Remove(f2.Name()); err != nil {
			l.Warnf("Failed to remove file %s %v", f2.Name(), err)
		}
	}()

	if _, err = f1.Write(b1); err != nil {
		return nil, err
	}

	if _, err = f2.Write(b2); err != nil {
		return nil, err
	}

	data, err := exec.CommandContext(ctx, "diff", "--label="+filepath.Join("old", path), "--label="+filepath.Join("new/", path), "-u", f1.Name(), f2.Name()).CombinedOutput()
	if len(data) > 0 {
		// diff exits with a non-zero status when the files don't match.
		// Ignore that failure as long as we get output.
		err = nil
	}

	return data, err
}
