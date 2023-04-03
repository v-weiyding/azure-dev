package cmd

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/cmd/middleware"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/pkg/account"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type upFlags struct {
	provisionFlags
	deployFlags
	global *internal.GlobalCommandOptions
	envFlag
}

func (u *upFlags) Bind(local *pflag.FlagSet, global *internal.GlobalCommandOptions) {
	u.envFlag.Bind(local, global)
	u.global = global

	u.provisionFlags.bindNonCommon(local, global)
	u.provisionFlags.setCommon(&u.envFlag)
	u.deployFlags.bindNonCommon(local, global)
	u.deployFlags.setCommon(&u.envFlag)
}

func newUpFlags(cmd *cobra.Command, global *internal.GlobalCommandOptions) *upFlags {
	flags := &upFlags{}
	flags.Bind(cmd.Flags(), global)

	return flags
}

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Provision Azure resources, and deploy your project with a single command.",
	}
}

type upAction struct {
	flags                      *upFlags
	env                        *environment.Environment
	accountManager             account.Manager
	packageActionInitializer   actions.ActionInitializer[*packageAction]
	provisionActionInitializer actions.ActionInitializer[*provisionAction]
	deployActionInitializer    actions.ActionInitializer[*deployAction]
	console                    input.Console
	runner                     middleware.MiddlewareContext
}

func newUpAction(
	flags *upFlags,
	env *environment.Environment,
	accountManager account.Manager,
	packageActionInitializer actions.ActionInitializer[*packageAction],
	provisionActionInitializer actions.ActionInitializer[*provisionAction],
	deployActionInitializer actions.ActionInitializer[*deployAction],
	console input.Console,
	runner middleware.MiddlewareContext,
) actions.Action {
	return &upAction{
		flags:                      flags,
		env:                        env,
		accountManager:             accountManager,
		packageActionInitializer:   packageActionInitializer,
		provisionActionInitializer: provisionActionInitializer,
		deployActionInitializer:    deployActionInitializer,
		console:                    console,
		runner:                     runner,
	}
}

func (u *upAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	if u.flags.provisionFlags.noProgress {
		fmt.Fprintln(
			u.console.Handles().Stderr,
			//nolint:lll
			output.WithWarningFormat("WARNING: The '--no-progress' flag is deprecated and will be removed in a future release."))
		// this flag actually isn't used by the provision command, we set it to false to hide the extra warning
		u.flags.provisionFlags.noProgress = false
	}

	if u.flags.deployFlags.serviceName != "" {
		fmt.Fprintln(
			u.console.Handles().Stderr,
			//nolint:lll
			output.WithWarningFormat("WARNING: The '--service' flag is deprecated and will be removed in a future release."))
	}

	err := provisioning.EnsureSubscriptionAndLocation(ctx, u.console, u.env, u.accountManager)
	if err != nil {
		return nil, err
	}

	packageAction, err := u.packageActionInitializer()
	if err != nil {
		return nil, err
	}
	packageOptions := &middleware.Options{CommandPath: "package"}
	_, err = u.runner.RunChildAction(ctx, packageOptions, packageAction)
	if err != nil {
		return nil, err
	}

	provision, err := u.provisionActionInitializer()
	if err != nil {
		return nil, err
	}

	provision.flags = &u.flags.provisionFlags
	provisionOptions := &middleware.Options{CommandPath: "provision"}
	_, err = u.runner.RunChildAction(ctx, provisionOptions, provision)
	if err != nil {
		return nil, err
	}

	// Print an additional newline to separate provision from deploy
	u.console.Message(ctx, "")

	deploy, err := u.deployActionInitializer()
	if err != nil {
		return nil, err
	}

	deploy.flags = &u.flags.deployFlags
	// move flag to args to avoid extra deprecation flag warning
	if deploy.flags.serviceName != "" {
		deploy.args = []string{deploy.flags.serviceName}
		deploy.flags.serviceName = ""
	}
	deployOptions := &middleware.Options{CommandPath: "deploy"}
	deployResult, err := u.runner.RunChildAction(ctx, deployOptions, deploy)
	if err != nil {
		return nil, err
	}

	return deployResult, nil
}

func getCmdUpHelpDescription(c *cobra.Command) string {
	return generateCmdHelpDescription(
		fmt.Sprintf("Executes the %s and %s commands in a single step.",
			output.WithHighLightFormat("azd provision"),
			output.WithHighLightFormat("azd deploy")), nil)
}
