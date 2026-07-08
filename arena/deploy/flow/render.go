package flow

import "github.com/AltairaLabs/PromptKit/runtime/deploy"

// ActionSymbol maps a plan Action to its diff glyph (Terraform-style).
func ActionSymbol(a deploy.Action) string {
	switch a { //nolint:exhaustive // ActionNoChange and unknown actions fall through to default
	case deploy.ActionCreate:
		return "+"
	case deploy.ActionUpdate:
		return "~"
	case deploy.ActionDelete:
		return "-"
	case deploy.ActionDrift:
		return "!"
	default: // ActionNoChange and unknown
		return " "
	}
}

// StatusSymbol maps an apply/destroy result status to its glyph.
func StatusSymbol(status string) string {
	switch status {
	case "created":
		return "+"
	case "updated":
		return "~"
	case "deleted":
		return "-"
	case "failed":
		return "!"
	default:
		return " "
	}
}
