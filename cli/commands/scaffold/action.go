package scaffold

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/shell"

	"github.com/gruntwork-io/terragrunt/terraform"

	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	"github.com/gruntwork-io/terragrunt/util"

	boilerplate_options "github.com/gruntwork-io/boilerplate/options"
	"github.com/gruntwork-io/boilerplate/templates"
	"github.com/gruntwork-io/boilerplate/variables"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terratest/modules/files"
	"github.com/hashicorp/go-getter"
)

const (
	moduleCount            = 2
	moduleAndTemplateCount = 3

	sourceUrlTypeHttps = "git-https"
	sourceUrlTypeGit   = "git-ssh"
	sourceGitSshUser   = "git"

	sourceUrlTypeVar    = "SourceUrlType"
	sourceGitSshUserVar = "SourceGitSshUser"
	refVar              = "Ref"
	// refParam - ?ref param from url
	refParam = "ref"

	moduleUrlPattern = `git::([^:]+)://([^/]+)(/.*)`
	moduleUrlParts   = 4

	defaultBoilerplateConfig = `
variables:

`
	defaultTerragruntTemplate = `
# This is a Terragrunt module generated by boilerplate.
terraform {
  source = "{{ .sourceUrl }}"
}

inputs = {
  # --------------------------------------------------------------------------------------------------------------------
  # Required input variables
  # --------------------------------------------------------------------------------------------------------------------
  {{ range .requiredVariables }}
  # Description: {{ .Description }}
  # Type: {{ .Type }}
  {{ .Name }} = {{ .DefaultValuePlaceholder }}  # TODO: fill in value
  {{ end }}

  # --------------------------------------------------------------------------------------------------------------------
  # Optional input variables
  # Uncomment the ones you wish to set
  # --------------------------------------------------------------------------------------------------------------------
  {{ range .optionalVariables }}
  # Description: {{ .Description }}
  # Type: {{ .Type }}
  # {{ .Name }} = {{ .DefaultValue }}
  {{ end }}
}
`
)

var moduleUrlRegex = regexp.MustCompile(moduleUrlPattern)

func Run(opts *options.TerragruntOptions) error {
	// download remote repo to local
	var moduleUrl = ""
	var templateUrl = ""
	// clean all temp dirs
	var dirsToClean []string
	defer func() {
		for _, dir := range dirsToClean {
			if err := os.RemoveAll(dir); err != nil {
				opts.Logger.Warnf("Failed to clean up dir %s: %v", dir, err)
			}
		}
	}()

	// scaffold only in empty directories
	if empty, err := util.IsDirectoryEmpty(opts.WorkingDir); !empty || err != nil {
		if err != nil {
			return err
		}
		return WorkingDirectoryNotEmptyError{
			dir: opts.WorkingDir,
		}
	}

	if len(opts.TerraformCliArgs) >= moduleCount {
		moduleUrl = opts.TerraformCliArgs[1]
	}

	if len(opts.TerraformCliArgs) >= moduleAndTemplateCount {
		templateUrl = opts.TerraformCliArgs[2]
	}

	tempDir, err := os.MkdirTemp("", "scaffold")
	if err != nil {
		return errors.WithStackTrace(err)
	}
	dirsToClean = append(dirsToClean, tempDir)

	// prepare variables
	vars, err := variables.ParseVars(opts.ScaffoldVars, opts.ScaffoldVarFiles)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// parse module url
	parsedModuleUrl, err := terraform.ToSourceUrl(moduleUrl, tempDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	moduleUrl = parsedModuleUrl.String()

	// rewrite module url, if required
	parsedModuleUrl, err = rewriteModuleUrl(opts, vars, moduleUrl)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// add ref to module url, if required
	parsedModuleUrl, err = addRefToModuleUrl(opts, parsedModuleUrl, vars)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// regenerate module url with all changes
	moduleUrl = parsedModuleUrl.String()

	// identify template url
	templateDir := ""
	if templateUrl != "" {
		// process template url if was passed
		parsedTemplateUrl, err := terraform.ToSourceUrl(templateUrl, tempDir)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		parsedTemplateUrl, err = rewriteTemplateUrl(opts, parsedTemplateUrl)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		// regenerate template url with all changes
		templateUrl = parsedTemplateUrl.String()

		templateDir, err = os.MkdirTemp("", "template")
		if err != nil {
			return errors.WithStackTrace(err)
		}
		dirsToClean = append(dirsToClean, templateDir)

		// downloading template
		opts.Logger.Infof("Using template from %s", templateUrl)
		if err := getter.GetAny(templateDir, templateUrl); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	opts.Logger.Infof("Scaffolding a new Terragrunt module %s %s to %s", moduleUrl, templateUrl, opts.WorkingDir)
	if err := getter.GetAny(tempDir, moduleUrl); err != nil {
		return errors.WithStackTrace(err)
	}

	// extract variables from module url
	inputs, err := config.ParseVariables(opts, tempDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// separate variables that require value and with default value
	var requiredVariables []*config.ParsedVariable
	var optionalVariables []*config.ParsedVariable

	for _, value := range inputs {
		if value.DefaultValue == "" {
			requiredVariables = append(requiredVariables, value)
		} else {
			optionalVariables = append(optionalVariables, value)
		}
	}
	opts.Logger.Debugf("Parsed %d required variables and %d optional variables", len(requiredVariables), len(optionalVariables))

	// run boilerplate

	// prepare boilerplate dir
	boilerplateDir := util.JoinPath(tempDir, util.DefaultBoilerplateDir)
	// use template dir as boilerplate dir
	if templateDir != "" {
		boilerplateDir = templateDir
	}

	// if boilerplate dir is not found, create one with default template
	if !files.IsExistingDir(boilerplateDir) {
		// no default boilerplate dir, create one
		boilerplateDir, err = os.MkdirTemp("", "boilerplate")
		if err != nil {
			return errors.WithStackTrace(err)
		}
		dirsToClean = append(dirsToClean, boilerplateDir)
		if err := os.WriteFile(util.JoinPath(boilerplateDir, "terragrunt.hcl"), []byte(defaultTerragruntTemplate), 0644); err != nil {
			return errors.WithStackTrace(err)
		}
		if err := os.WriteFile(util.JoinPath(boilerplateDir, "boilerplate.yml"), []byte(defaultBoilerplateConfig), 0644); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	// add additional variables
	vars["requiredVariables"] = requiredVariables
	vars["optionalVariables"] = optionalVariables

	vars["sourceUrl"] = moduleUrl

	opts.Logger.Infof("Running boilerplate generation to %s", opts.WorkingDir)
	boilerplateOpts := &boilerplate_options.BoilerplateOptions{
		TemplateFolder:  boilerplateDir,
		OutputFolder:    opts.WorkingDir,
		OnMissingKey:    boilerplate_options.DefaultMissingKeyAction,
		OnMissingConfig: boilerplate_options.DefaultMissingConfigAction,
		Vars:            vars,
		DisableShell:    true,
		NonInteractive:  opts.NonInteractive,
	}
	emptyDep := variables.Dependency{}
	if err := templates.ProcessTemplate(boilerplateOpts, boilerplateOpts, emptyDep); err != nil {
		return errors.WithStackTrace(err)
	}

	opts.Logger.Infof("Running fmt on generated code %s", opts.WorkingDir)
	if err := hclfmt.Run(opts); err != nil {
		return errors.WithStackTrace(err)
	}

	opts.Logger.Info("Scaffolding completed")

	return nil
}

// rewriteModuleUrl rewrites module url to git ssh if required
func rewriteModuleUrl(opts *options.TerragruntOptions, vars map[string]interface{}, moduleUrl string) (*url.URL, error) {
	var updatedModuleUrl = moduleUrl
	sourceUrlType := sourceUrlTypeHttps
	if value, found := vars[sourceUrlTypeVar]; found {
		sourceUrlType = fmt.Sprintf("%s", value)
	}

	// rewrite module url
	parsedUrl, err := parseUrl(opts, moduleUrl)
	if err == nil {
		opts.Logger.Warnf("Failed to parse module url %s", moduleUrl)
		parsedModuleUrl, err := terraform.ToSourceUrl(updatedModuleUrl, opts.WorkingDir)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		return parsedModuleUrl, nil
	}
	// try to rewrite module url if is https and is requested to be git
	if parsedUrl.scheme == "https" && sourceUrlType == sourceUrlTypeGit {
		gitUser := sourceGitSshUser
		if value, found := vars[sourceGitSshUserVar]; found {
			gitUser = fmt.Sprintf("%s", value)
		}
		path := strings.TrimPrefix(parsedUrl.path, "/")
		updatedModuleUrl = fmt.Sprintf("%s@%s:%s", gitUser, parsedUrl.host, path)
	}

	parsedModuleUrl, err := terraform.ToSourceUrl(updatedModuleUrl, opts.WorkingDir)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return parsedModuleUrl, nil
}

// rewriteTemplateUrl rewrites template url with reference to tag
func rewriteTemplateUrl(opts *options.TerragruntOptions, parsedTemplateUrl *url.URL) (*url.URL, error) {
	var updatedTemplateUrl = parsedTemplateUrl
	var templateParams = updatedTemplateUrl.Query()
	ref := templateParams.Get(refParam)
	if ref == "" {
		rootSourceUrl, _, err := terraform.SplitSourceUrl(updatedTemplateUrl, opts.Logger)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		tag, err := shell.GitLastReleaseTag(opts, rootSourceUrl)
		if err != nil || tag == "" {
			opts.Logger.Warnf("Failed to find last release tag for URL %s, so will not add a ref param to the URL", rootSourceUrl)
		} else {
			templateParams.Add(refParam, tag)
			updatedTemplateUrl.RawQuery = templateParams.Encode()
		}
	}
	return updatedTemplateUrl, nil
}

// addRefToModuleUrl adds ref to module url if is passed through variables or find it from git tags
func addRefToModuleUrl(opts *options.TerragruntOptions, parsedModuleUrl *url.URL, vars map[string]interface{}) (*url.URL, error) {
	var moduleUrl = parsedModuleUrl
	// append ref to source url, if is passed through variables or find it from git tags
	params := moduleUrl.Query()
	refReplacement, refVarPassed := vars[refVar]
	if refVarPassed {
		params.Set(refParam, fmt.Sprintf("%s", refReplacement))
		moduleUrl.RawQuery = params.Encode()
	}
	ref := params.Get(refParam)
	if ref == "" {
		// if ref is not passed, find last release tag
		rootSourceUrl, _, err := terraform.SplitSourceUrl(moduleUrl, opts.Logger)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}

		tag, err := shell.GitLastReleaseTag(opts, rootSourceUrl)
		if err != nil || tag == "" {
			opts.Logger.Warnf("Failed to find last release tag for %s", rootSourceUrl)
		} else {
			params.Add(refParam, tag)
			moduleUrl.RawQuery = params.Encode()
		}
	}
	return moduleUrl, nil
}

// parseUrl parses module url to scheme, host and path
func parseUrl(opts *options.TerragruntOptions, moduleUrl string) (*parsedUrl, error) {
	matches := moduleUrlRegex.FindStringSubmatch(moduleUrl)
	if len(matches) != moduleUrlParts {
		opts.Logger.Warnf("Failed to parse module url %s", moduleUrl)
		return nil, failedToParseUrlError{}
	}
	return &parsedUrl{
		scheme: matches[1],
		host:   matches[2],
		path:   matches[3],
	}, nil
}

type parsedUrl struct {
	scheme string
	host   string
	path   string
}

type failedToParseUrlError struct {
}

func (err failedToParseUrlError) Error() string {
	return "Failed to parse Url."
}

type WorkingDirectoryNotEmptyError struct {
	dir string
}

func (err WorkingDirectoryNotEmptyError) Error() string {
	return fmt.Sprintf("The working directory %s is not empty.", err.dir)
}
