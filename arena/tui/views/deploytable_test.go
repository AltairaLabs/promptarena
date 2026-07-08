package views

import (
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

func TestDeployRowsFromResults(t *testing.T) {
	rows := DeployRowsFromResults([]*deploy.ResourceResult{
		{Type: "agent_runtime", Name: "bot", Status: "created"},
		{Type: "a2a_endpoint", Name: "ep", Status: "failed", Detail: "quota"},
	})
	if len(rows) != 2 || rows[0].Symbol != "+" || rows[1].Symbol != "!" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	out := stripANSIForTest(RenderDeployResources(rows, 80))
	if !strings.Contains(out, "agent_runtime") || !strings.Contains(out, "bot") {
		t.Fatalf("table missing content:\n%s", out)
	}
}

func TestDeployRowsFromStatus(t *testing.T) {
	rows := DeployRowsFromStatus([]deploy.ResourceStatus{
		{Type: "agent_runtime", Name: "bot", Status: "healthy"},
		{Type: "a2a_endpoint", Name: "ep", Status: "unhealthy", Detail: "5xx"},
		{Type: "secret", Name: "s", Status: "missing"},
	})
	if len(rows) != 3 || rows[0].Symbol != "✓" || rows[1].Symbol != "✗" || rows[2].Symbol != "?" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	out := stripANSIForTest(RenderDeployResources(rows, 80))
	if !strings.Contains(out, "a2a_endpoint") || !strings.Contains(out, "5xx") {
		t.Fatalf("table missing content:\n%s", out)
	}
}
