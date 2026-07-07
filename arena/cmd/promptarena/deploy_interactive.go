package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the arena pack to a target environment",
	Long: `Deploy the arena pack to a target environment using a deploy adapter.

When run without a subcommand, deploy executes a plan followed by apply.

Subcommands:
  plan      Preview what would be deployed
  apply     Apply the deployment
  status    Show current deployment status
  destroy   Tear down a deployment
  refresh   Refresh local state from live environment
  import    Import a pre-existing resource

Examples:
  promptarena deploy --env production
  promptarena deploy plan --env staging
  promptarena deploy apply --env staging
  promptarena deploy status --env production
  promptarena deploy destroy --env staging
  promptarena deploy refresh --env production
  promptarena deploy import agent_runtime my-agent container-abc123`,
	RunE: runDeploy,
}

var (
	deployEnv      string
	deployConfig   string
	deployPackFile string
)

func init() {
	rootCmd.AddCommand(deployCmd)

	deployCmd.PersistentFlags().StringVarP(&deployEnv, "env", "e", "", "Target environment")
	deployCmd.PersistentFlags().StringVar(&deployConfig, "config", "arena.yaml", "Config file path")
	deployCmd.PersistentFlags().StringVar(&deployPackFile, "pack", "",
		"Optional pre-compiled *.pack.json to deploy (default: compile from --config)")

	deployCmd.AddCommand(deployPlanCmd)
	deployCmd.AddCommand(deployApplyCmd)
	deployCmd.AddCommand(deployStatusCmd)
	deployCmd.AddCommand(deployDestroyCmd)
	deployCmd.AddCommand(deployRefreshCmd)
	deployCmd.AddCommand(deployImportCmd)
}

// deployConfigureDocsURL points users at the how-to for setting up a deploy
// configuration when one is missing or incomplete.
const deployConfigureDocsURL = "https://promptkit.altairalabs.ai/arena/how-to/deploy/configure/"

// deployOptions builds a flow.Options from the deploy command's persistent
// flags, for handing off to the flow package's config/pack resolution.
func deployOptions() flow.Options {
	return flow.Options{ConfigPath: deployConfig, Env: deployEnv, PackFile: deployPackFile}
}

// loadDeployConfig loads the arena config and returns the deploy section.
func loadDeployConfig() (*arenaconfig.DeployConfig, error) {
	_, deployCfg, err := flow.LoadConfig(deployOptions())
	return deployCfg, err
}

// importArgCount is the number of positional arguments for the import command.
const importArgCount = 3

// connectAdapter discovers the adapter binary and returns a connected client.
func connectAdapter(provider, projectDir string) (*deploy.AdapterClient, error) {
	printAdapterPath(provider, projectDir)
	return flow.Connect(context.Background(), provider, projectDir)
}

// printAdapterPath shows which adapter binary a command will use. It mirrors
// connectAdapter's informational line for commands that connect via an
// already-open flow.Session (flow.Open) rather than connectAdapter directly.
func printAdapterPath(provider, projectDir string) {
	if path, found := flow.AdapterInstalled(provider, projectDir); found {
		fmt.Printf("  Adapter:     %s\n", path)
	}
}

// printDeployWarnings renders non-blocking adapter warnings to stderr with a
// "⚠" prefix. It is a no-op when there are none.
func printDeployWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", w)
	}
}

// printPlan displays a deployment plan to the user.
func printPlan(plan *deploy.PlanResponse) {
	fmt.Println()
	printDeployWarnings(plan.Warnings)
	fmt.Printf("Plan: %s\n", plan.Summary)
	fmt.Println()
	if len(plan.Changes) == 0 {
		fmt.Println("  No changes required.")
		return
	}
	for _, c := range plan.Changes {
		line := fmt.Sprintf("  %s %s.%s", flow.ActionSymbol(c.Action), c.Type, c.Name)
		if c.Detail != "" {
			line += " (" + c.Detail + ")"
		}
		fmt.Println(line)
	}
	fmt.Println()
}

// printDeployEvent renders a streaming apply/destroy event to stdout. Apply and
// Destroy share the same event shape (type, message, resource), so both
// callbacks delegate here to surface per-resource progress live.
func printDeployEvent(eventType, message string, res *deploy.ResourceResult) {
	switch eventType {
	case "resource":
		if res != nil {
			line := fmt.Sprintf("  %s %s.%s (%s)",
				flow.StatusSymbol(res.Status), res.Type, res.Name, res.Status)
			if res.Detail != "" {
				line += ": " + res.Detail
			}
			fmt.Println(line)
		}
	case "error":
		fmt.Printf("  ! %s\n", message)
	}
}

// printStatus displays a deployment status to the user.
func printStatus(status *deploy.StatusResponse) {
	fmt.Printf("  Status: %s\n", status.Status)
	if len(status.Resources) > 0 {
		fmt.Println()
		for _, r := range status.Resources {
			line := fmt.Sprintf("  %s.%s: %s", r.Type, r.Name, r.Status)
			if r.Detail != "" {
				line += " — " + r.Detail
			}
			fmt.Println(line)
		}
	}
	fmt.Println()
}

// --- deploy (plan + apply) ---

func runDeploy(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("Deploying with provider %q to environment %q\n", sess.Deploy.Provider, sess.Env)
	fmt.Println()
	printAdapterPath(sess.Deploy.Provider, projectDir)

	// Step 1: Plan.
	fmt.Println()
	fmt.Println("Step 1: Planning deployment...")

	plan, planReq, err := sess.Plan(ctx)
	if err != nil {
		return err
	}
	printPlan(plan)

	// Step 2: Apply.
	fmt.Println("Step 2: Applying deployment...")

	if err := sess.Apply(ctx, planReq, func(e *deploy.ApplyEvent) error {
		printDeployEvent(e.Type, e.Message, e.Resource)
		return nil
	}); err != nil {
		return err
	}

	fmt.Println("Deployment complete.")
	return nil
}
