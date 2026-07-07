package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
)

var deployRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh local state from the live environment",
	Long: `Query the deploy adapter for the current state of all resources
and update local state to match reality. Use this to detect drift
between local state and cloud resources.

Examples:
  promptarena deploy refresh --env production
  promptarena deploy refresh --env staging`,
	RunE: runDeployRefresh,
}

func runDeployRefresh(cmd *cobra.Command, args []string) error {
	deployCfg, err := loadDeployConfig()
	if err != nil {
		return err
	}

	env := flow.ResolveEnv(deployOptions())
	projectDir, _ := os.Getwd()
	ctx := context.Background()

	// Acquire deploy lock.
	unlock, err := flow.Lock(projectDir)
	if err != nil {
		return err
	}
	defer unlock()

	fmt.Printf("Refreshing state for environment: %s (provider: %s)\n", env, deployCfg.Provider)

	configJSON, err := flow.MergedConfigJSON(deployCfg, env, deployConfig)
	if err != nil {
		return err
	}

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

	client, err := connectAdapter(deployCfg.Provider, projectDir)
	if err != nil {
		return err
	}
	defer client.Close()

	status, err := client.Status(ctx, &deploy.StatusRequest{
		DeployConfig: configJSON,
		Environment:  env,
		PriorState:   priorState.State,
	})
	if err != nil {
		return fmt.Errorf("refresh failed: %w", err)
	}

	if status.State != "" {
		priorState.State = status.State
	}
	priorState.LastRefreshed = time.Now().UTC().Format(time.RFC3339)
	if err := stateStore.Save(priorState); err != nil {
		return fmt.Errorf("failed to save refreshed state: %w", err)
	}

	fmt.Println()
	printStatus(status)
	fmt.Println("State refreshed successfully.")
	return nil
}
