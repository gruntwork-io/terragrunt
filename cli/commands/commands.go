// Package commands represents CLI commands.
package commands

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/cli/commands/info"
	"github.com/gruntwork-io/terragrunt/cli/commands/stack"
	"github.com/gruntwork-io/terragrunt/options"

	"github.com/gruntwork-io/terragrunt/cli/commands/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclvalidate"
	helpCmd "github.com/gruntwork-io/terragrunt/cli/commands/help"
	versionCmd "github.com/gruntwork-io/terragrunt/cli/commands/version"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog"
	execCmd "github.com/gruntwork-io/terragrunt/cli/commands/exec"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	outputmodulegroups "github.com/gruntwork-io/terragrunt/cli/commands/output-module-groups"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	runCmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	validateinputs "github.com/gruntwork-io/terragrunt/cli/commands/validate-inputs"
	"github.com/gruntwork-io/terragrunt/internal/cli"
)

// Command category names.
const (
	// MainCommandsCategoryName represents primary Terragrunt operations like run, exec.
	MainCommandsCategoryName = "Main commands"
	// CatalogCommandsCategoryName represents commands for managing Terragrunt catalogs.
	CatalogCommandsCategoryName = "Catalog commands"
	// DiscoveryCommandsCategoryName represents commands for discovering Terragrunt configurations.
	DiscoveryCommandsCategoryName = "Discovery commands"
	// ConfigurationCommandsCategoryName represents commands for managing Terragrunt configurations.
	ConfigurationCommandsCategoryName = "Configuration commands"
	// ShortcutsCommandsCategoryName represents OpenTofu-specific shortcut commands.
	ShortcutsCommandsCategoryName = "OpenTofu shortcuts"
)

// New returns the set of Terragrunt commands, grouped into categories.
// Categories are ordered in increments of 10 for easy insertion of new categories.
func New(opts *options.TerragruntOptions) cli.Commands {
	mainCommands := cli.Commands{
		runCmd.NewCommand(opts),  // run
		runall.NewCommand(opts),  // run-all
		stack.NewCommand(opts),   // stack
		graph.NewCommand(opts),   // graph
		execCmd.NewCommand(opts), // exec
	}.SetCategory(
		&cli.Category{
			Name:  MainCommandsCategoryName,
			Order: 10, //nolint: mnd
		},
	)

	catalogCommands := cli.Commands{
		catalog.NewCommand(opts),  // catalog
		scaffold.NewCommand(opts), // scaffold
	}.SetCategory(
		&cli.Category{
			Name:  CatalogCommandsCategoryName,
			Order: 20, //nolint: mnd
		},
	)

	discoveryCommands := cli.Commands{
		find.NewCommand(opts), // find
	}.SetCategory(
		&cli.Category{
			Name:  DiscoveryCommandsCategoryName,
			Order: 30, //nolint: mnd
		},
	)

	configurationCommands := cli.Commands{
		graphdependencies.NewCommand(opts),  // graph-dependencies
		outputmodulegroups.NewCommand(opts), // output-module-groups
		validateinputs.NewCommand(opts),     // validate-inputs
		hclvalidate.NewCommand(opts),        // hclvalidate
		hclfmt.NewCommand(opts),             // hclfmt
		info.NewCommand(opts),               // info
		terragruntinfo.NewCommand(opts),     // terragrunt-info
		renderjson.NewCommand(opts),         // render-json
		helpCmd.NewCommand(opts),            // help (hidden)
		versionCmd.NewCommand(opts),         // version (hidden)
		awsproviderpatch.NewCommand(opts),   // aws-provider-patch (hidden)
	}.SetCategory(
		&cli.Category{
			Name:  ConfigurationCommandsCategoryName,
			Order: 40, //nolint: mnd
		},
	)

	shortcutsCommands := NewShortcutsCommands(opts).SetCategory(
		&cli.Category{
			Name:  ShortcutsCommandsCategoryName,
			Order: 50, //nolint: mnd
		},
	)

	allCommands := mainCommands.
		Merge(catalogCommands...).
		Merge(discoveryCommands...).
		Merge(configurationCommands...).
		Merge(shortcutsCommands...).
		Merge(NewDeprecatedCommands(opts)...)

	return allCommands
}
