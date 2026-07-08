package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/AltairaLabs/promptarena/arena/deploy/flow"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

// deployConfigCmd groups commands that operate on the deploy configuration
// itself (as opposed to a live deployment).
var deployConfigCmd = &cobra.Command{
	Use:   deployConfigCmdName,
	Short: "Manage the deploy configuration",
	Long: `Manage the deploy section of your arena config.

Subcommands:
  import   Merge an exported deploy profile into the config and validate it`,
}

var (
	deployConfigImportProvider     string
	deployConfigImportSkipValidate bool
)

var deployConfigImportCmd = &cobra.Command{
	Use:   "import <profile-file|->",
	Short: "Import a deploy profile into the deploy config and validate it",
	Long: `Merge an exported deploy profile fragment into the deploy config
(under the provider's config: block), then validate the result against the
adapter's validate_config RPC.

The profile is a JSON or YAML mapping — typically exported by your deploy
provider (e.g. Omnia) — containing connection details, a scoped token, and
discovered providers/skills. Pass "-" to read the profile from stdin.

The merge is surgical: only deploy.config is touched, and the rest of your
config file (comments, key order, other sections) is preserved. Existing keys
are overridden by the profile; nested mappings are deep-merged. If the deploy
section or its config block is absent it is created, with the provider set from
--provider.

Examples:
  promptarena deploy config import omnia-profile.json
  promptarena deploy config import - < omnia-profile.yaml
  promptarena deploy config import profile.json --provider omnia
  promptarena deploy config import profile.json --skip-validate`,
	Args: cobra.ExactArgs(1),
	RunE: runDeployConfigImport,
}

func init() {
	deployCmd.AddCommand(deployConfigCmd)
	deployConfigCmd.AddCommand(deployConfigImportCmd)

	deployConfigImportCmd.Flags().StringVar(
		&deployConfigImportProvider, "provider", "omnia",
		"Provider to set when the deploy section is created",
	)
	deployConfigImportCmd.Flags().BoolVar(
		&deployConfigImportSkipValidate, "skip-validate", false,
		"Skip validating the merged config against the adapter",
	)
}

// deployConfigCmdName is the "config" subcommand name (shared with the
// --config flag string elsewhere in the package).
const deployConfigCmdName = "config"

// readProfile reads a deploy profile fragment from path (or stdin when path is
// "-") and parses it as YAML, which is a superset of JSON. The profile must be
// a mapping.
func readProfile(path string, stdin io.Reader) (map[string]interface{}, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		if stdin == nil {
			stdin = os.Stdin
		}
		data, err = io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read profile from stdin: %w", err)
		}
	} else {
		data, err = os.ReadFile(path) //nolint:gosec // path is a user-supplied flag
		if err != nil {
			return nil, fmt.Errorf("failed to read profile file %s: %w", path, err)
		}
	}

	var profile map[string]interface{}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse profile (expects a JSON or YAML mapping): %w", err)
	}
	if profile == nil {
		return nil, fmt.Errorf("profile is empty or not a mapping")
	}
	return profile, nil
}

func runDeployConfigImport(cmd *cobra.Command, args []string) error {
	profilePath := args[0]

	profile, err := readProfile(profilePath, cmd.InOrStdin())
	if err != nil {
		return err
	}

	if err := flow.MergeProfileIntoConfigFile(deployConfig, profile, deployConfigImportProvider); err != nil {
		return err
	}
	fmt.Printf("Imported profile into %s (merged under deploy.config)\n", deployConfig)

	if deployConfigImportSkipValidate {
		fmt.Println("Skipping validation (--skip-validate).")
		return nil
	}

	return validateDeployConfig()
}

// validateDeployConfig loads the (just-merged) deploy config and validates it
// against the adapter's validate_config RPC.
func validateDeployConfig() error {
	deployCfg, err := loadDeployConfig()
	if err != nil {
		return err
	}

	env := flow.ResolveEnv(deployOptions())
	projectDir, _ := os.Getwd()
	ctx := context.Background()

	configJSON, err := flow.MergedConfigJSON(deployCfg, env, deployConfig)
	if err != nil {
		return err
	}

	fmt.Printf("Validating config against %q adapter...\n", deployCfg.Provider)
	client, err := connectAdapter(deployCfg.Provider, projectDir)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.ValidateConfig(ctx, &deploy.ValidateRequest{Config: configJSON})
	if err != nil {
		return fmt.Errorf("validate_config failed: %w", err)
	}
	if !resp.Valid {
		fmt.Println()
		fmt.Println("Config is INVALID:")
		for _, e := range resp.Errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("deploy config validation failed (%d error(s))", len(resp.Errors))
	}

	printDeployWarnings(resp.Warnings)
	fmt.Println("Config is valid.")
	return nil
}
