package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
)

var deployPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Preview the deployment plan",
	Long: `Show what resources would be created, updated, or destroyed
without making any changes. The plan is saved so that 'deploy apply'
can execute it without re-planning.

Examples:
  promptarena deploy plan --env staging
  promptarena deploy plan --config custom.yaml`,
	RunE: runDeployPlan,
}

func runDeployPlan(cmd *cobra.Command, args []string) error {
	opts := deployOptions()
	projectDir, _ := os.Getwd()
	ctx := context.Background()

	sess, err := flow.Open(ctx, opts)
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()

	fmt.Printf("Deploy plan for environment: %s (provider: %s)\n", sess.Env, sess.Deploy.Provider)
	printAdapterPath(sess.Deploy.Provider, projectDir)

	plan, planReq, err := sess.Plan(ctx)
	if err != nil {
		return err
	}

	printPlan(plan)

	// Save plan for later apply.
	if err := sess.SavePlan(plan, planReq); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}
	fmt.Println("Plan saved. Run 'promptarena deploy apply' to execute.")
	return nil
}
