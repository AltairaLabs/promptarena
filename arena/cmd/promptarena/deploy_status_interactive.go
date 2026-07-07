package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
)

var deployStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show deployment status",
	Long: `Show the current status of a deployment in the target environment.

Examples:
  promptarena deploy status --env production
  promptarena deploy status --env staging`,
	RunE: runDeployStatus,
}

func runDeployStatus(cmd *cobra.Command, args []string) error {
	opts := deployOptions()
	deployCfg, err := loadDeployConfig()
	if err != nil {
		return err
	}

	env := flow.ResolveEnv(opts)
	projectDir, _ := os.Getwd()
	ctx := context.Background()

	fmt.Printf("Deployment status for environment: %s (provider: %s)\n", env, deployCfg.Provider)

	// Validate the deploy config before checking local state, so a broken
	// --config surfaces its error even when there is no prior deployment.
	if _, err := flow.MergedConfigJSON(deployCfg, env, deployConfig); err != nil {
		return err
	}

	// Peek at local state before connecting the adapter — status should be a
	// cheap, local check when nothing has been deployed yet (and shouldn't
	// require the adapter binary to be installed just to say so).
	stateStore := deploy.NewStateStore(projectDir)
	priorState, err := stateStore.Load()
	if err != nil {
		return fmt.Errorf("failed to load deploy state: %w", err)
	}
	if priorState == nil {
		fmt.Println()
		fmt.Println("  No deployment state found. Run 'promptarena deploy' first.")
		return nil
	}

	sess, err := flow.Open(ctx, opts)
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()

	printAdapterPath(sess.Deploy.Provider, projectDir)

	status, err := sess.Status(ctx)
	if err != nil {
		return fmt.Errorf("status check failed: %w", err)
	}

	fmt.Println()
	printStatus(status)

	fmt.Printf("  Last deployed: %s\n", priorState.LastDeployed)
	fmt.Printf("  Pack checksum: %s\n", priorState.PackChecksum)
	return nil
}
