package flow

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

func TestLinksFromResults_CollectsInOrder(t *testing.T) {
	results := []*deploy.ResourceResult{
		{Type: "agent_runtime", Name: "support", Links: []deploy.ResourceLink{
			{Label: "Console", URL: "https://ws.example/agents/support", Rel: "console"},
		}},
		{Type: "agent_runtime", Name: "billing", Links: []deploy.ResourceLink{
			{Label: "Console", URL: "https://ws.example/agents/billing", Rel: "console"},
		}},
	}

	got := LinksFromResults(results)

	if len(got) != 2 {
		t.Fatalf("LinksFromResults() returned %d links, want 2: %+v", len(got), got)
	}
	if got[0].URL != "https://ws.example/agents/support" {
		t.Errorf("got[0].URL = %q, want the first result's link", got[0].URL)
	}
	if got[1].URL != "https://ws.example/agents/billing" {
		t.Errorf("got[1].URL = %q, want the second result's link", got[1].URL)
	}
}

// An adapter may attach the same shared link (a workspace dashboard, say) to
// every resource it reports. The operator should see it once.
func TestLinksFromResults_DeduplicatesRepeatedURLs(t *testing.T) {
	shared := deploy.ResourceLink{Label: "Dashboard", URL: "https://ws.example/", Rel: "dashboard"}
	results := []*deploy.ResourceResult{
		{Type: "agent_runtime", Name: "support", Links: []deploy.ResourceLink{shared}},
		{Type: "agent_runtime", Name: "billing", Links: []deploy.ResourceLink{shared}},
	}

	if got := LinksFromResults(results); len(got) != 1 {
		t.Errorf("LinksFromResults() returned %d links, want the duplicate URL collapsed to 1: %+v",
			len(got), got)
	}
}

func TestLinksFromStatus_CollectsDeploymentWideThenPerResource(t *testing.T) {
	status := &deploy.StatusResponse{
		Links: []deploy.ResourceLink{
			{Label: "Dashboard", URL: "https://ws.example/", Rel: "dashboard"},
		},
		Resources: []deploy.ResourceStatus{
			{Type: "agent_runtime", Name: "support", Links: []deploy.ResourceLink{
				{Label: "Console", URL: "https://ws.example/agents/support", Rel: "console"},
			}},
		},
	}

	got := LinksFromStatus(status)

	if len(got) != 2 {
		t.Fatalf("LinksFromStatus() returned %d links, want 2: %+v", len(got), got)
	}
	if got[0].URL != "https://ws.example/" {
		t.Errorf("got[0].URL = %q, want the deployment-wide link first", got[0].URL)
	}
	if got[1].URL != "https://ws.example/agents/support" {
		t.Errorf("got[1].URL = %q, want the per-resource link second", got[1].URL)
	}
}

// An adapter may legitimately report the same console URL both deployment-wide
// and against the resource it belongs to. The operator should see it once.
func TestLinksFromStatus_DeduplicatesRepeatedURLs(t *testing.T) {
	status := &deploy.StatusResponse{
		Links: []deploy.ResourceLink{
			{Label: "Console", URL: "https://ws.example/agents/support", Rel: "console"},
		},
		Resources: []deploy.ResourceStatus{
			{Type: "agent_runtime", Name: "support", Links: []deploy.ResourceLink{
				{Label: "Console", URL: "https://ws.example/agents/support", Rel: "console"},
			}},
		},
	}

	if got := LinksFromStatus(status); len(got) != 1 {
		t.Errorf("LinksFromStatus() returned %d links, want the duplicate URL collapsed to 1: %+v",
			len(got), got)
	}
}

func TestLinksFromStatus_NilStatusReturnsNone(t *testing.T) {
	if got := LinksFromStatus(nil); len(got) != 0 {
		t.Errorf("LinksFromStatus(nil) = %+v, want no links", got)
	}
}

func TestLinksFromResults_NoLinksReturnsNone(t *testing.T) {
	results := []*deploy.ResourceResult{
		{Type: "agent_runtime", Name: "support"},
		{Type: "promptpack", Name: "pack"},
	}

	if got := LinksFromResults(results); len(got) != 0 {
		t.Errorf("LinksFromResults() = %+v, want no links when no adapter supplied any", got)
	}
}
