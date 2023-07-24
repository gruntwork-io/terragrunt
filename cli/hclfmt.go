package cli

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/mattn/go-zglob"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// runHCLFmt recursively looks for hcl files in the directory tree starting at workingDir, and formats them
// based on the language style guides provided by Hashicorp. This is done using the official hcl2 library.
func runHCLFmt(terragruntOptions *options.TerragruntOptions) error {

	workingDir := terragruntOptions.WorkingDir
	targetFile := terragruntOptions.HclFile

	// handle when option specifies a particular file
	if targetFile != "" {
		if !filepath.IsAbs(targetFile) {
			targetFile = util.JoinPath(workingDir, targetFile)
		}
		terragruntOptions.Logger.Debugf("Formatting hcl file at: %s.", targetFile)
		return formatTgHCL(terragruntOptions, targetFile)
	}

	terragruntOptions.Logger.Debugf("Formatting hcl files from the directory tree %s.", terragruntOptions.WorkingDir)
	// zglob normalizes paths to "/"
	tgHclFiles, err := zglob.Glob(util.JoinPath(workingDir, "**", "*.hcl"))
	if err != nil {
		return err
	}

	filteredTgHclFiles := []string{}
	for _, fname := range tgHclFiles {
		// Ignore any files that are in the .terragrunt-cache
		if !util.ListContainsElement(strings.Split(fname, "/"), util.TerragruntCacheDir) {
			filteredTgHclFiles = append(filteredTgHclFiles, fname)
		} else {
			terragruntOptions.Logger.Debugf("%s was ignored due to being in the terragrunt cache", fname)
		}
	}

	terragruntOptions.Logger.Debugf("Found %d hcl files", len(filteredTgHclFiles))

	var formatErrors *multierror.Error
	for _, tgHclFile := range filteredTgHclFiles {
		err := formatTgHCL(terragruntOptions, tgHclFile)
		if err != nil {
			formatErrors = multierror.Append(formatErrors, err)
		}
	}

	return formatErrors.ErrorOrNil()
}

// formatTgHCL uses the hcl2 library to format the hcl file. This will attempt to parse the HCL file first to
// ensure that there are no syntax errors, before attempting to format it.
func formatTgHCL(terragruntOptions *options.TerragruntOptions, tgHclFile string) error {
	terragruntOptions.Logger.Debugf("Formatting %s", tgHclFile)

	info, err := os.Stat(tgHclFile)
	if err != nil {
		terragruntOptions.Logger.Errorf("Error retrieving file info of %s", tgHclFile)
		return err
	}

	contentsStr, err := util.ReadFileAsString(tgHclFile)
	if err != nil {
		terragruntOptions.Logger.Errorf("Error reading %s", tgHclFile)
		return err
	}
	contents := []byte(contentsStr)

	err = checkErrors(terragruntOptions.Logger, contents, tgHclFile)
	if err != nil {
		terragruntOptions.Logger.Errorf("Error parsing %s", tgHclFile)
		return err
	}

	newContents := hclwrite.Format(contents)

	fileUpdated := !bytes.Equal(newContents, contents)

	if terragruntOptions.Diff && fileUpdated {
		diff, err := bytesDiff(contents, newContents, tgHclFile)
		if err != nil {
			terragruntOptions.Logger.Errorf("Failed to generate diff for %s", tgHclFile)
			return err
		}
		_, err = fmt.Fprintf(terragruntOptions.Writer, "%s\n", diff)
		if err != nil {
			terragruntOptions.Logger.Errorf("Failed to print diff for %s", tgHclFile)
			return err
		}
	}

	if terragruntOptions.Check && fileUpdated {
		return fmt.Errorf("Invalid file format %s", tgHclFile)
	}

	if fileUpdated {
		terragruntOptions.Logger.Infof("%s was updated", tgHclFile)
		return ioutil.WriteFile(tgHclFile, newContents, info.Mode())
	}

	return nil
}

// checkErrors takes in the contents of a hcl file and looks for syntax errors.
func checkErrors(logger *logrus.Entry, contents []byte, tgHclFile string) error {
	parser := hclparse.NewParser()
	_, diags := parser.ParseHCL(contents, tgHclFile)
	diagWriter := util.GetDiagnosticsWriter(logger, parser)
	diagWriter.WriteDiagnostics(diags)
	if diags.HasErrors() {
		return diags
	}
	return nil
}

// bytesDiff uses GNU diff to display the differences between the contents of HCL file before and after formatting
func bytesDiff(b1, b2 []byte, path string) ([]byte, error) {
	f1, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, err
	}
	defer func() {
		f1.Close()
		os.Remove(f1.Name())
	}()

	f2, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, err
	}
	defer func() {
		f2.Close()
		os.Remove(f2.Name())
	}()
	if _, err := f1.Write(b1); err != nil {
		return nil, err
	}
	if _, err := f2.Write(b2); err != nil {
		return nil, err
	}
	data, err := exec.Command("diff", "--label="+filepath.Join("old", path), "--label="+filepath.Join("new/", path), "-u", f1.Name(), f2.Name()).CombinedOutput()
	if len(data) > 0 {
		// diff exits with a non-zero status when the files don't match.
		// Ignore that failure as long as we get output.
		err = nil
	}
	return data, err
}
