package cli

import (
	"io/ioutil"
	"os"

	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/hashicorp/hcl2/hclwrite"
	"github.com/mattn/go-zglob"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// runHCLFmt recursively looks for terragrunt.hcl files in the directory tree starting at workingDir, and formats them
// based on the language style guides provided by Hashicorp. This is done using the official hcl2 library.
func runHCLFmt(terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Formatting terragrunt.hcl files from the directory tree %s.", terragruntOptions.WorkingDir)

	workingDir := terragruntOptions.WorkingDir
	tgHclFiles, err := zglob.Glob(util.JoinPath(workingDir, "**", "*.hcl"))
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Found %d terragrunt.hcl files", len(tgHclFiles))

	for _, tgHclFile := range tgHclFiles {
		err := formatTgHCL(terragruntOptions, tgHclFile)
		if err != nil {
			return err
		}
	}

	return nil
}

// formatTgHCL uses the hcl2 library to format the terragrunt.hcl file. This will attempt to parse the HCL file first to
// ensure that there are no syntax errors, before attempting to format it.
func formatTgHCL(terragruntOptions *options.TerragruntOptions, tgHclFile string) error {
	terragruntOptions.Logger.Printf("Formatting %s", tgHclFile)

	info, err := os.Stat(tgHclFile)
	if err != nil {
		terragruntOptions.Logger.Printf("Error retrieving file info of %s", tgHclFile)
		return err
	}

	contentsStr, err := util.ReadFileAsString(tgHclFile)
	if err != nil {
		terragruntOptions.Logger.Printf("Error reading %s", tgHclFile)
		return err
	}
	contents := []byte(contentsStr)

	err = checkErrors(contents, tgHclFile)
	if err != nil {
		terragruntOptions.Logger.Printf("Error parsing %s", tgHclFile)
		return err
	}

	newContents := hclwrite.Format(contents)
	return ioutil.WriteFile(tgHclFile, newContents, info.Mode())
}

// checkErrors takes in the contents of a terragrunt.hcl file and looks for syntax errors.
func checkErrors(contents []byte, tgHclFile string) error {
	parser := hclparse.NewParser()
	_, diags := parser.ParseHCL(contents, tgHclFile)
	diagWriter := util.GetDiagnosticsWriter(parser)
	diagWriter.WriteDiagnostics(diags)
	if diags.HasErrors() {
		return diags
	}
	return nil
}
