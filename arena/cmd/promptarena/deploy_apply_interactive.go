package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
)

var deployApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the deployment",
	Long: `Apply the deployment plan to the target environment.
If a saved plan exists (from 'deploy plan'), it will be used.
Otherwise, a new plan is generated before applying.

Examples:
  promptarena deploy apply --env staging
  promptarena deploy apply --env production --config custom.yaml`,
	RunE: runDeployApply,
}

func runDeployApply(cmd *cobra.Command, args []string) error {
	opts := deployOptions()
	projectDir, _ := os.Getwd()
	ctx := context.Background()

	// Acquire deploy lock.
	unlock, err := flow.Lock(projectDir)
	if err != nil {
		return err
	}
	defer unlock()

	sess, err := flow.Open(ctx, opts)
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()

	fmt.Printf("Applying deployment to environment: %s (provider: %s)\n", sess.Env, sess.Deploy.Provider)

	// Check for a saved plan.
	var planReq *deploy.PlanRequest
	savedPlan, err := sess.LoadPlan()
	if err != nil {
		return fmt.Errorf("failed to load saved plan: %w", err)
	}
	usedSavedPlan := false
	if sess.PlanIsFresh(savedPlan) {
		fmt.Println("  Using saved plan.")
		planReq = savedPlan.Request
		usedSavedPlan = true
	} else {
		if savedPlan != nil {
			fmt.Println("  Saved plan is stale (pack or environment changed), re-planning...")
		}
		planReq, err = sess.PlanRequest(ctx)
		if err != nil {
			return err
		}
	}

	printAdapterPath(sess.Deploy.Provider, projectDir)

	// Surface the plan's non-blocking advisories before applying — the same
	// warnings the plan command shows — from the saved plan, or a fresh plan when
	// re-planning. Apply streams resource events but has no Warnings channel.
	printDeployWarnings(applyWarnings(ctx, sess.Client, usedSavedPlan, savedPlan, planReq))

	if err := sess.Apply(ctx, planReq, func(e *deploy.ApplyEvent) error {
		printDeployEvent(e.Type, e.Message, e.Resource)
		return nil
	}); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Apply complete.")
	return nil
}

// planWarner fetches a plan for its non-blocking warnings. It's satisfied by
// *deploy.AdapterClient; an interface so applyWarnings is unit-testable.
type planWarner interface {
	Plan(context.Context, *deploy.PlanRequest) (*deploy.PlanResponse, error)
}

// applyWarnings returns the plan-time advisories to surface before an apply: the
// reused saved plan's warnings, otherwise a fresh plan's. A plan error is
// non-fatal here — apply proceeds and runs its own validation.
func applyWarnings(
	ctx context.Context, client planWarner,
	usedSavedPlan bool, savedPlan *deploy.SavedPlan, planReq *deploy.PlanRequest,
) []string {
	if usedSavedPlan && savedPlan != nil && savedPlan.Plan != nil {
		return savedPlan.Plan.Warnings
	}
	if plan, err := client.Plan(ctx, planReq); err == nil {
		return plan.Warnings
	}
	return nil
}
