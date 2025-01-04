// Package run contains the logic for interacting with OpenTofu/Terraform.
package run

import (
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

const (
	CommandName = "run"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Run an OpenTofu/Terraform command. Shortcuts for common `run` commands are provided below.",
		UsageText:   "terragrunt run [options] -- <tofu/terraform command>",
		Description: "Run a command, passing arguments to a wrapped tofu/terraform binary.\n\nThis is the explicit, and most flexible form of running an IaC update with Terragrunt. Shortcuts can be found in \"terragrunt --help\" for common use-cases.",
		Examples: []string{
			"# Run a plan\nterragrunt run -- plan\n# Shortcut:\n# terragrunt plan",
			"# Run output with -json flag\nterragrunt run -- output -json\n# Shortcut:\n# terragrunt output -json",
			"# Run a plan against a Stack of configurations in the current directory\nterragrunt run --all -- plan",
		},
		Flags:                NewFlags(opts),
		ErrorOnUndefinedFlag: true,
		Action:               Action(opts),
		Hidden:               true,
	}
}

func Action(opts *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if opts.TerraformCommand == tf.CommandNameDestroy {
			opts.CheckDependentModules = !opts.NoDestroyDependenciesCheck
		}

		if !opts.DisableCommandValidation && !collections.ListContainsElement(tf.CommandNames, opts.TerraformCommand) {
			if strings.HasSuffix(opts.TerraformPath, options.TerraformDefaultPath) {
				return errors.New(WrongTerraformCommand(opts.TerraformCommand))
			} else {
				// We default to tofu if the terraform path does not end in Terraform
				return errors.New(WrongTofuCommand(opts.TerraformCommand))
			}
		}

		return Run(ctx.Context, opts.OptionsFromContext(ctx))
	}
}
