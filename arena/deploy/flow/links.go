package flow

import "github.com/AltairaLabs/promptarena/deploy"

// LinksFromResults collects the operator-facing links an adapter attached to
// applied resources, in the order the adapter reported them. An adapter may
// attach the same shared link to several resources, so repeats are collapsed
// to the first mention.
func LinksFromResults(results []*deploy.ResourceResult) []deploy.ResourceLink {
	var out []deploy.ResourceLink
	seen := map[string]bool{}
	for _, r := range results {
		out = appendUniqueLinks(out, seen, r.Links)
	}
	return out
}

// LinksFromStatus collects the operator-facing links from a status response:
// deployment-wide links first, then each resource's own. An adapter may report
// the same URL in both places, so repeats are collapsed to the first mention.
func LinksFromStatus(status *deploy.StatusResponse) []deploy.ResourceLink {
	if status == nil {
		return nil
	}
	var out []deploy.ResourceLink
	seen := map[string]bool{}
	out = appendUniqueLinks(out, seen, status.Links)
	for i := range status.Resources {
		out = appendUniqueLinks(out, seen, status.Resources[i].Links)
	}
	return out
}

// appendUniqueLinks appends the links whose URL has not already been seen,
// recording each one it keeps.
func appendUniqueLinks(
	out []deploy.ResourceLink, seen map[string]bool, links []deploy.ResourceLink,
) []deploy.ResourceLink {
	for _, l := range links {
		if seen[l.URL] {
			continue
		}
		seen[l.URL] = true
		out = append(out, l)
	}
	return out
}
