package arenaconfig

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/AltairaLabs/PromptKit/runtime/v2/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// Taxonomy frameworks built from the pack governance an arena config declares
// (pack_metadata.governance and each agent's governance). An objective maps to
// one as obligation:<id> or misuse:<slug>, so the objectives report shows how
// often each declared obligation's controls held, and lists the declared
// obligations and foreseeable misuses no objective tested.
const (
	// TaxonomyObligation names a governance obligation by its id.
	TaxonomyObligation = "obligation"
	// TaxonomyMisuse names a foreseeable_misuse entry by its slug: the text
	// lowercased, with each run of other characters replaced by a hyphen.
	TaxonomyMisuse = "misuse"
)

// MisuseSlug is the id a foreseeable_misuse entry is named by in a taxonomy,
// such as "refunds-outside-policy" for "Refunds outside policy". Apostrophes
// are dropped, so "customer's" is "customers".
func MisuseSlug(text string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(text) {
		switch {
		case r == '\'' || r == '’':
			continue
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			hyphen = false
		default:
			hyphen = true
		}
	}
	return b.String()
}

// GovernanceFrameworks returns the obligation and misuse frameworks of the
// governance the config declares, or none when it declares none. An id
// declared at pack level and again by an agent keeps the first declaration.
func (c *Config) GovernanceFrameworks() []TaxonomyFramework {
	var decls []*prompt.Governance
	if c.PackMetadata != nil && c.PackMetadata.Governance != nil {
		decls = append(decls, c.PackMetadata.Governance)
	}
	if c.Agents != nil {
		names := make([]string, 0, len(c.Agents.Members))
		for name := range c.Agents.Members {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if m := c.Agents.Members[name]; m != nil && m.Governance != nil {
				decls = append(decls, m.Governance)
			}
		}
	}

	obligations := TaxonomyFramework{Name: TaxonomyObligation, Title: "Governance obligations",
		Titles: map[string]string{}}
	misuse := TaxonomyFramework{Name: TaxonomyMisuse, Title: "Foreseeable misuse", Titles: map[string]string{}}
	for _, g := range decls {
		for _, o := range g.Obligations {
			if o == nil || o.ID == "" || slices.Contains(obligations.IDs, o.ID) {
				continue
			}
			obligations.IDs = append(obligations.IDs, o.ID)
			obligations.Titles[o.ID] = obligationTitle(o.Obligation, controlNames(o.Controls))
		}
		for _, text := range g.ForeseeableMisuse {
			slug := MisuseSlug(text)
			if slug == "" || slices.Contains(misuse.IDs, slug) {
				continue
			}
			misuse.IDs = append(misuse.IDs, slug)
			misuse.Titles[slug] = text
		}
	}

	var out []TaxonomyFramework
	for _, f := range []TaxonomyFramework{obligations, misuse} {
		if len(f.IDs) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// controlNames renders an obligation's controls as kind:name, such as
// validator:refund-guard.
func controlNames(controls []*packspec.ObligationControl) []string {
	var out []string
	for _, ctl := range controls {
		if ctl == nil {
			continue
		}
		for _, kv := range [][2]string{
			{"validator", ctl.Validator}, {"eval", ctl.Eval}, {"field", ctl.Field}, {"external", ctl.External},
		} {
			if kv[1] != "" {
				out = append(out, kv[0]+":"+kv[1])
			}
		}
	}
	return out
}

func obligationTitle(text string, controls []string) string {
	if len(controls) == 0 {
		return text
	}
	return fmt.Sprintf("%s (controls: %s)", text, strings.Join(controls, ", "))
}

// validateGovernanceTaxonomy checks every obligation: and misuse: entry in a
// scenario objective's taxonomy, after dataset rows fill it in, against the
// governance the config declares.
func (c *Config) validateGovernanceTaxonomy() error {
	frameworks := c.GovernanceFrameworks()
	ids := map[string][]string{}
	for _, f := range frameworks {
		ids[f.Name] = f.IDs
	}
	scenarioIDs := make([]string, 0, len(c.LoadedScenarios))
	for id := range c.LoadedScenarios {
		scenarioIDs = append(scenarioIDs, id)
	}
	sort.Strings(scenarioIDs)
	for _, sid := range scenarioIDs {
		s := c.LoadedScenarios[sid]
		for _, filled := range scenarioVariants(s) {
			if err := checkGovernanceEntries(filled, ids); err != nil {
				return fmt.Errorf("scenario %s: %w", sid, err)
			}
		}
	}
	return nil
}

// scenarioVariants is the scenario once per dataset row, or as written when it
// has no rows.
func scenarioVariants(s *Scenario) []*Scenario {
	if s.Data == nil || len(s.Data.Rows) == 0 {
		return []*Scenario{s}
	}
	out := make([]*Scenario, 0, len(s.Data.Rows))
	for _, row := range s.Data.Rows {
		out = append(out, s.ForDataRow(row))
	}
	return out
}

func checkGovernanceEntries(s *Scenario, ids map[string][]string) error {
	for i := range s.Turns {
		o := s.Turns[i].Objective
		if o == nil {
			continue
		}
		for j, e := range o.Taxonomy {
			name, id, _ := strings.Cut(e, ":")
			if name != TaxonomyObligation && name != TaxonomyMisuse {
				continue
			}
			where := fmt.Sprintf("turns[%d].objective.taxonomy[%d]", i, j)
			known := ids[name]
			if len(known) == 0 {
				return fmt.Errorf("%s: %q names pack governance, but the config's pack_metadata.governance "+
					"and agents declare no %s", where, e, governanceField(name))
			}
			if !slices.Contains(known, id) {
				return fmt.Errorf("%s: governance declares no %s %q; declared: %s",
					where, name, id, strings.Join(known, ", "))
			}
		}
	}
	return nil
}

func governanceField(name string) string {
	if name == TaxonomyObligation {
		return "obligations"
	}
	return "foreseeable_misuse"
}
