package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AltairaLabs/promptarena/arena/generate"
)

func runGenerate(cmd *cobra.Command, _ []string) error {
	req, err := buildRequest(cmd)
	if err != nil {
		return err
	}

	res, err := generate.Generate(context.Background(), req)
	if err != nil {
		return err
	}

	for _, s := range res.Skipped {
		fmt.Fprintf(os.Stderr, "warning: skipping session %s: %s\n", s.SessionID, s.Reason)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	for _, d := range res.Decisions {
		fmt.Println(d.String())
	}
	if res.Deduplicated > 0 {
		fmt.Printf("Deduplicated: removed %d duplicate(s), %d session(s) remaining.\n", res.Deduplicated, len(res.Scenarios))
	}
	if len(res.Scenarios) == 0 {
		fmt.Println("No sessions produced a scenario.")
		return nil
	}

	outputDir, _ := cmd.Flags().GetString("output")
	paths, err := generate.WriteScenarios(outputDir, res.Scenarios)
	for _, p := range paths {
		fmt.Printf("Generated: %s\n", p)
	}
	if err != nil {
		return err
	}
	fmt.Printf("\nGenerated %d scenario file(s).\n", len(paths))
	return nil
}
