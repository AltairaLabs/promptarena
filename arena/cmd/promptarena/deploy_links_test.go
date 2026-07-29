package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

func TestPrintDeployLinks_PrintsLabelAndURL(t *testing.T) {
	var buf bytes.Buffer

	printDeployLinks(&buf, []deploy.ResourceLink{
		{Label: "Console", URL: "https://ws.example/agents/support", Rel: "console"},
	})

	out := buf.String()
	if !strings.Contains(out, "Console") {
		t.Errorf("output %q does not name the link", out)
	}
	if !strings.Contains(out, "https://ws.example/agents/support") {
		t.Errorf("output %q does not contain the URL", out)
	}
}

// The headless path is the one used over SSH and in CI, where there is no
// browser at all — the URL is the entire deliverable, so it must be printed
// whole and on its own line, ready to copy.
func TestPrintDeployLinks_URLIsAloneOnItsLine(t *testing.T) {
	url := "https://acme.omnia.example/workspaces/prod/agents/support-triage"
	var buf bytes.Buffer

	printDeployLinks(&buf, []deploy.ResourceLink{{Label: "Console", URL: url, Rel: "console"}})

	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, url) {
			if strings.TrimSpace(line) != url {
				t.Errorf("URL line is %q, want the bare URL so it can be copied cleanly", line)
			}
			return
		}
	}
	t.Errorf("URL never appeared in output %q", buf.String())
}

func TestPrintDeployLinks_NoLinksPrintsNothing(t *testing.T) {
	var buf bytes.Buffer

	printDeployLinks(&buf, nil)

	if buf.Len() != 0 {
		t.Errorf("printDeployLinks(nil) wrote %q, want nothing at all", buf.String())
	}
}

func TestPrintDeployLinks_PrintsEveryLink(t *testing.T) {
	var buf bytes.Buffer

	printDeployLinks(&buf, []deploy.ResourceLink{
		{Label: "Console", URL: "https://ws.example/agents/support"},
		{Label: "Logs", URL: "https://ws.example/agents/support/logs"},
	})

	out := buf.String()
	for _, want := range []string{
		"https://ws.example/agents/support",
		"https://ws.example/agents/support/logs",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q is missing link %q", out, want)
		}
	}
}
