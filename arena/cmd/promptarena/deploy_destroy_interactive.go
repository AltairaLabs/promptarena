package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/v2/arena/deploy/flow"

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
	deployCfg, err := loadDeployConfig()
	if err != nil {
		return err
	}

	env := flow.ResolveEnv(deployOptions())
	projectDir, _ := os.Getwd()
	ctx := context.Background()

	// Acquire deploy lock.
	unlock, err := flow.Lock(projectDir, env)
	if err != nil {
		return err
	}
	defer unlock()

	fmt.Printf("Destroying deployment in environment: %s (provider: %s)\n", env, deployCfg.Provider)

	configJSON, err := flow.MergedConfigJSON(deployCfg, env, deployConfigPath())
	if err != nil {
		return err
	}

	// Only this environment's state is ever sent to the adapter: destroying
	// one environment must never hand it another environment's resources.
	stateStore, err := openEnvState(projectDir, env)
	if err != nil {
		return err
	}
	priorState, err := stateStore.Load()
	if err != nil {
		return fmt.Errorf("failed to load deploy state: %w", err)
	}

	if priorState == nil {
		fmt.Println()
		fmt.Printf("  No deployment state found for environment %q. Nothing to destroy.\n", env)
		return nil
	}

	client, err := connectAdapter(deployCfg.Provider, projectDir)
	if err != nil {
		return err
	}
	defer client.Close()

	err = client.Destroy(ctx, &deploy.DestroyRequest{
		DeployConfig: configJSON,
		Environment:  env,
		PriorState:   priorState.State,
	}, func(e *deploy.DestroyEvent) error {
		printDeployEvent(e.Type, e.Message, e.Resource)
		return nil
	})
	if err != nil {
		return fmt.Errorf("destroy failed: %w", err)
	}

	if err := stateStore.Delete(); err != nil {
		return fmt.Errorf("failed to remove deploy state: %w", err)
	}
	// A plan saved before the destroy describes resources that no longer exist.
	_ = stateStore.DeletePlan()

	fmt.Println()
	fmt.Println("Deployment destroyed.")
	return nil
}
