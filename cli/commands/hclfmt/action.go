package hclfmt

import (
	"bytes"
	goErrors "errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"

	"github.com/sirupsen/logrus"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/mattn/go-zglob"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// Run runs the hclfmt command.
func Run(opts *options.TerragruntOptions) error {
	workingDir := opts.WorkingDir
	targetFile := opts.HclFile

	// handle when option specifies a particular file
	if targetFile != "" {
		if !filepath.IsAbs(targetFile) {
			targetFile = util.JoinPath(workingDir, targetFile)
		}

		opts.Logger.Debugf("Formatting hcl file at: %s.", targetFile)

		return formatTgHCL(opts, targetFile)
	}

	opts.Logger.Debugf("Formatting hcl files from the directory tree %s.", opts.WorkingDir)

	// zglob normalizes paths to "/"
	tgHclFiles, err := zglob.Glob(util.JoinPath(workingDir, "**", "*.hcl"))
	if err != nil {
		return fmt.Errorf("error finding hcl files: %w", err)
	}

	filteredTgHclFiles := []string{}

	for _, fname := range tgHclFiles {
		skipFile := false
		// Ignore any files that are in the cache or scaffold dir
		if util.ListContainsElement(strings.Split(fname, "/"), util.TerragruntCacheDir) {
			skipFile = true
		}

		if util.ListContainsElement(strings.Split(fname, "/"), util.DefaultBoilerplateDir) {
			skipFile = true
		}

		if skipFile {
			opts.Logger.Debugf("%s was ignored", fname)
		} else {
			filteredTgHclFiles = append(filteredTgHclFiles, fname)
		}
	}

	opts.Logger.Debugf("Found %d hcl files", len(filteredTgHclFiles))

	var formatErrors *multierror.Error

	for _, tgHclFile := range filteredTgHclFiles {
		err := formatTgHCL(opts, tgHclFile)
		if err != nil {
			formatErrors = multierror.Append(formatErrors, err)
		}
	}

	err = formatErrors.ErrorOrNil()
	if err != nil {
		return fmt.Errorf("error formatting hcl files: %w", err)
	}

	return nil
}

var (
	// ErrInvalidFileFormat is returned when the file format is invalid.
	ErrInvalidFileFormat = goErrors.New("invalid file format")
)

// formatTgHCL uses the hcl2 library to format the hcl file. This will attempt to parse the HCL file first to
// ensure that there are no syntax errors, before attempting to format it.
func formatTgHCL(opts *options.TerragruntOptions, tgHclFile string) error {
	opts.Logger.Debugf("Formatting %s", tgHclFile)

	info, err := os.Stat(tgHclFile)
	if err != nil {
		opts.Logger.Errorf("Error retrieving file info of %s", tgHclFile)

		return fmt.Errorf("error retrieving file info of %s: %w", tgHclFile, err)
	}

	contentsStr, err := util.ReadFileAsString(tgHclFile)
	if err != nil {
		opts.Logger.Errorf("Error reading %s", tgHclFile)

		return fmt.Errorf("error reading %s: %w", tgHclFile, err)
	}

	contents := []byte(contentsStr)

	err = checkErrors(opts.Logger, opts.DisableLogColors, contents, tgHclFile)
	if err != nil {
		opts.Logger.Errorf("Error parsing %s", tgHclFile)

		return err
	}

	newContents := hclwrite.Format(contents)

	fileUpdated := !bytes.Equal(newContents, contents)

	if opts.Diff && fileUpdated {
		diff, err := bytesDiff(opts, contents, newContents, tgHclFile)
		if err != nil {
			opts.Logger.Errorf("Failed to generate diff for %s", tgHclFile)

			return fmt.Errorf("failed to generate diff for %s: %w", tgHclFile, err)
		}

		_, err = fmt.Fprintf(opts.Writer, "%s\n", diff)

		if err != nil {
			opts.Logger.Errorf("Failed to print diff for %s", tgHclFile)

			return fmt.Errorf("failed to print diff for %s: %w", tgHclFile, err)
		}
	}

	if opts.Check && fileUpdated {
		// return fmt.Errorf("Invalid file format %s", tgHclFile)
		return fmt.Errorf("%w: %s", ErrInvalidFileFormat, tgHclFile)
	}

	if fileUpdated {
		opts.Logger.Infof("%s was updated", tgHclFile)

		err = os.WriteFile(tgHclFile, newContents, info.Mode())
		if err != nil {
			return fmt.Errorf("error writing to %s: %w", tgHclFile, err)
		}

		return nil
	}

	return nil
}

// checkErrors takes in the contents of a hcl file and looks for syntax errors.
func checkErrors(logger *logrus.Entry, disableColor bool, contents []byte, tgHclFile string) error {
	parser := hclparse.NewParser()
	_, diags := parser.ParseHCL(contents, tgHclFile)

	writer := &util.LogWriter{Logger: logger, Level: logrus.ErrorLevel}
	diagWriter := util.GetDiagnosticsWriter(writer, parser, disableColor)
	err := diagWriter.WriteDiagnostics(diags)

	if err != nil {
		return fmt.Errorf("error writing diagnostics: %w", errors.WithStackTrace(err))
	}

	if diags.HasErrors() {
		return diags
	}

	return nil
}

// bytesDiff uses GNU diff to display the differences between the contents of HCL file before and after formatting.
func bytesDiff(opts *options.TerragruntOptions, b1, b2 []byte, path string) ([]byte, error) {
	f1, err := os.CreateTemp("", "")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	defer func() {
		if err := f1.Close(); err != nil {
			opts.Logger.Warnf("Failed to close file %s %v", f1.Name(), err)
		}

		if err := os.Remove(f1.Name()); err != nil {
			opts.Logger.Warnf("Failed to remove file %s %v", f1.Name(), err)
		}
	}()

	f2, err := os.CreateTemp("", "")

	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	defer func() {
		if err := f2.Close(); err != nil {
			opts.Logger.Warnf("Failed to close file %s %v", f2.Name(), err)
		}

		if err := os.Remove(f2.Name()); err != nil {
			opts.Logger.Warnf("Failed to remove file %s %v", f2.Name(), err)
		}
	}()

	if _, err := f1.Write(b1); err != nil {
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	if _, err := f2.Write(b2); err != nil {
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	data, err := exec.Command(
		"diff",
		"--label="+filepath.Join("old", path),
		"--label="+filepath.Join("new/", path),
		"-u",
		f1.Name(),
		f2.Name(),
	).CombinedOutput()

	if len(data) > 0 {
		// diff exits with a non-zero status when the files don't match.
		// Ignore that failure as long as we get output.
		err = nil
	}

	return data, err
}
