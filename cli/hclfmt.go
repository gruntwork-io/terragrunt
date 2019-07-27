package cli

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/hashicorp/hcl2/hclwrite"
	"golang.org/x/crypto/ssh/terminal"
)

func runHCLFmt(terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Formatting terragrunt.hcl files in the current directory tree.")

	tgHclFiles, err := findTerragruntHclFiles(terragruntOptions)
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

func findTerragruntHclFiles(terragruntOptions *options.TerragruntOptions) ([]string, error) {
	tgHclFiles := []string{}

	cwd, err := os.Getwd()
	if err != nil {
		return tgHclFiles, err
	}
	terragruntOptions.Logger.Printf("Walking directory tree in %s to look for terragrunt.hcl files", cwd)

	err = filepath.Walk(
		cwd,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if filepath.Base(path) == "terragrunt.hcl" && !info.IsDir() {
				tgHclFiles = append(tgHclFiles, path)
			}
			return nil
		},
	)
	return tgHclFiles, err
}

func formatTgHCL(terragruntOptions *options.TerragruntOptions, tgHclFile string) error {
	terragruntOptions.Logger.Printf("Formatting %s", tgHclFile)

	info, err := os.Stat(tgHclFile)
	if err != nil {
		return err
	}

	contents, err := readAllFromFile(tgHclFile)
	if err != nil {
		return err
	}

	err := checkErrors(contents, tgHclFile)
	if err != nil {
		terragruntOptions.Logger.Printf("Error parsing %s", tgHclFile)
		return err
	}

	newContents := hclwrite.Format(contents)
	return ioutil.WriteFile(tgHclFile, newContents, info.Mode())
}

func readAllFromFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(f)
}

func getDiagnosticsWriter(parser hclparse.Parser) hcl.DiagnosticWriter {
	termColor := terminal.IsTerminal(int(os.Stderr.Fd()))
	termWidth, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		termWidth = 80
	}
	return hcl.NewDiagnosticTextWriter(os.Stderr, parser.Files(), uint(termWidth), termColor)
}

func checkErrors(contents string, tgHclFile string) error {
	parser := hclparse.NewParser()
	_, diags := parser.ParseHCL(contents, tgHclFile)
	diagWriter := getDiagnosticsWriter(parser)
	diagWriter.WriteDiagnostics(diags)
	if diags.HasErrors() {
		return diags
	}
	return nil
}
