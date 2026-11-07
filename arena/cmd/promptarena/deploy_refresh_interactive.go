package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/v2/deploy"
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
	ec, err := openEnvCommand("Refreshing state for environment:")
	if err != nil {
		return err
	}
	defer ec.unlock()
	ctx := context.Background()

	if ec.state == nil {
		fmt.Println()
		fmt.Printf("  No deployment state found for environment %q. Run 'promptarena deploy --env %s' first.\n",
			ec.env, ec.env)
		return nil
	}

	client, err := connectAdapter(ec.cfg.Provider, ec.projectDir)
	if err != nil {
		return err
	}
	defer client.Close()

	status, err := client.Status(ctx, &deploy.StatusRequest{
		DeployConfig: ec.configJSON,
		Environment:  ec.env,
		PriorState:   ec.state.State,
	})
	if err != nil {
		return fmt.Errorf("refresh failed: %w", err)
	}

	if status.State != "" {
		ec.state.State = status.State
	}
	ec.state.LastRefreshed = time.Now().UTC().Format(time.RFC3339)
	if err := ec.store.Save(ec.state); err != nil {
		return fmt.Errorf("failed to save refreshed state: %w", err)
	}

	fmt.Println()
	printStatus(status)
	fmt.Println("State refreshed successfully.")
	return nil
}
