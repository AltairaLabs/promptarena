package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/v2/deploy"
)

var deployDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Tear down a deployment",
	Long: `Destroy all resources associated with a deployment in the target environment.

This is a destructive operation. Use with caution.

Examples:
  promptarena deploy destroy --env staging
  promptarena deploy destroy --env production`,
	RunE: runDeployDestroy,
}

func runDeployDestroy(cmd *cobra.Command, args []string) error {
	// Only this environment's state is ever sent to the adapter: destroying
	// one environment must never hand it another environment's resources.
	ec, err := openEnvCommand("Destroying deployment in environment:")
	if err != nil {
		return err
	}
	defer ec.unlock()
	ctx := context.Background()

	if ec.state == nil {
		fmt.Println()
		fmt.Printf("  No deployment state found for environment %q. Nothing to destroy.\n", ec.env)
		return nil
	}

	client, err := connectAdapter(ec.cfg.Provider, ec.projectDir)
	if err != nil {
		return err
	}
	defer client.Close()

	err = client.Destroy(ctx, &deploy.DestroyRequest{
		DeployConfig: ec.configJSON,
		Environment:  ec.env,
		PriorState:   ec.state.State,
	}, func(e *deploy.DestroyEvent) error {
		printDeployEvent(e.Type, e.Message, e.Resource)
		return nil
	})
	if err != nil {
		return fmt.Errorf("destroy failed: %w", err)
	}

	if err := ec.store.Delete(); err != nil {
		return fmt.Errorf("failed to remove deploy state: %w", err)
	}
	// A plan saved before the destroy describes resources that no longer exist.
	_ = ec.store.DeletePlan()

	fmt.Println()
	fmt.Println("Deployment destroyed.")
	return nil
}
