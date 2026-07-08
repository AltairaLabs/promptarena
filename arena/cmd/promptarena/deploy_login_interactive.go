package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/arena/deploy/flow"
)

var deployLoginProvider string

var deployLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate in the browser and write the deploy config",
	Long: `Open a browser to the provider, authenticate, and write the resulting
deploy profile (endpoint, workspace, providers, skills) into the arena config.
The scoped token is stored in ~/.promptarena/credentials — never in the config
file.

The provider must ship an adapter that supports login. The provider is taken
from --provider, or from the existing deploy section of the config.

Examples:
  promptarena deploy login --provider omnia
  promptarena deploy login                 # provider read from arena.yaml`,
	RunE: runDeployLogin,
}

func init() {
	deployCmd.AddCommand(deployLoginCmd)
	deployLoginCmd.Flags().StringVar(&deployLoginProvider, "provider", "",
		"Adapter provider to log in with (default: from the config's deploy section)")
}

// resolveLoginProvider returns the provider to log in with: the --provider flag
// wins; otherwise the existing deploy.provider in the config is used.
func resolveLoginProvider() (string, error) {
	if deployLoginProvider != "" {
		return deployLoginProvider, nil
	}
	if _, err := os.Stat(deployConfig); err == nil {
		if cfg, derr := loadDeployConfig(); derr == nil && cfg.Provider != "" {
			return cfg.Provider, nil
		}
	}
	return "", fmt.Errorf("no provider specified; pass --provider (e.g. --provider omnia)")
}

func runDeployLogin(_ *cobra.Command, _ []string) error {
	provider, err := resolveLoginProvider()
	if err != nil {
		return err
	}

	hooks := flow.LoginHooks{
		OnStatus: func(msg string) { fmt.Println(msg) },
		OnAuthorizeURL: func(url string) {
			fmt.Printf("  If it doesn't open, visit:\n  %s\n", url)
		},
	}

	if err := flow.Login(context.Background(), provider, deployOptions(), hooks); err != nil {
		return err
	}

	fmt.Printf("\nLogin complete. Wrote the deploy profile to %s\n", deployConfig)
	fmt.Println("The token was stored in ~/.promptarena/credentials (not in the config file).")
	return nil
}
