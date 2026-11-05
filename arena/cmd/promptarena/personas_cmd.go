package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/v2/arena/personas"
)

var (
	personasJSONFlag  bool
	personasDirFlag   string
	personasForceFlag bool
)

var personasCmd = &cobra.Command{
	Use:   "personas",
	Short: "Copy built-in self-play personas into a project",
	Long: `Built-in personas ship with promptarena and are copied into your project
as ordinary persona YAML, which you then edit like any other.

Examples:
  promptarena personas list
  promptarena personas show crescendo
  promptarena personas add crescendo --dir personas`,
}

var personasListCmd = &cobra.Command{
	Use:   examplesListUse,
	Short: "List the built-in personas",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return writePersonasList(cmd.OutOrStdout(), personasJSONFlag)
	},
}

var personasShowCmd = &cobra.Command{
	Use:   showNameUse,
	Short: "Print a built-in persona's YAML",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := personas.Read(args[0])
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	},
}

var personasAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Copy a built-in persona into the project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return addPersona(cmd.OutOrStdout(), args[0], personasDirFlag, personasForceFlag)
	},
}

func init() {
	rootCmd.AddCommand(personasCmd)
	personasCmd.AddCommand(personasListCmd, personasShowCmd, personasAddCmd)
	personasListCmd.Flags().BoolVar(&personasJSONFlag, "json", false, "Output JSON")
	personasAddCmd.Flags().StringVar(&personasDirFlag, "dir", "personas", "Directory to write the persona into")
	personasAddCmd.Flags().BoolVar(&personasForceFlag, "force", false, "Replace an existing file")
}

func writePersonasList(w io.Writer, asJSON bool) error {
	entries, err := personas.List()
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	tw := tabwriter.NewWriter(w, tabMinWidth, tabWidth, tabPadding, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tMODE\tDESCRIPTION"); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Name, e.Mode, e.Description); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// addPersona writes the persona and prints the config entry that loads it.
func addPersona(w io.Writer, name, dir string, force bool) error {
	path, err := personas.Write(dir, name, force)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, `Wrote %s

Add it to your arena config:

  self_play:
    personas:
      - file: %s

and use it on a self-play turn with persona: %s
`, path, filepath.ToSlash(path), name)
	return err
}
