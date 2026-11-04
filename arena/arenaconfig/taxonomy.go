package arenaconfig

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// TaxonomyFramework is a risk framework whose ids an objective's taxonomy
// can name. Arena knows each id's title, checks that an id exists, and lists
// the ids a run didn't test.
type TaxonomyFramework struct {
	// Name is the framework's short name, the prefix before the colon.
	Name string
	// Title is the framework's full name.
	Title string
	// IDs lists the framework's ids in order, with Titles giving each one's
	// title.
	IDs    []string
	Titles map[string]string
}

// TaxonomyFrameworks are the frameworks Arena knows. A taxonomy entry with
// another prefix is reported as written.
var TaxonomyFrameworks = []TaxonomyFramework{
	newTaxonomyFramework("owasp-llm", "OWASP Top 10 for LLM Applications 2025", [][2]string{
		{"LLM01", "Prompt Injection"},
		{"LLM02", "Sensitive Information Disclosure"},
		{"LLM03", "Supply Chain"},
		{"LLM04", "Data and Model Poisoning"},
		{"LLM05", "Improper Output Handling"},
		{"LLM06", "Excessive Agency"},
		{"LLM07", "System Prompt Leakage"},
		{"LLM08", "Vector and Embedding Weaknesses"},
		{"LLM09", "Misinformation"},
		{"LLM10", "Unbounded Consumption"},
	}),
	newTaxonomyFramework("owasp-agentic", "OWASP Top 10 for Agentic Applications 2026", [][2]string{
		{"ASI01", "Agent Goal Hijack"},
		{"ASI02", "Tool Misuse and Exploitation"},
		{"ASI03", "Identity and Privilege Abuse"},
		{"ASI04", "Agentic Supply Chain Vulnerabilities"},
		{"ASI05", "Unexpected Code Execution"},
		{"ASI06", "Memory and Context Poisoning"},
		{"ASI07", "Insecure Inter-Agent Communication"},
		{"ASI08", "Cascading Failures"},
		{"ASI09", "Human-Agent Trust Exploitation"},
		{"ASI10", "Rogue Agents"},
	}),
}

func newTaxonomyFramework(name, title string, ids [][2]string) TaxonomyFramework {
	f := TaxonomyFramework{Name: name, Title: title, Titles: make(map[string]string, len(ids))}
	for _, id := range ids {
		f.IDs = append(f.IDs, id[0])
		f.Titles[id[0]] = id[1]
	}
	return f
}

// TaxonomyFrameworkOf returns the known framework an entry such as
// "owasp-llm:LLM01" belongs to, and the id within it.
func TaxonomyFrameworkOf(entry string) (*TaxonomyFramework, string) {
	name, id, _ := strings.Cut(entry, ":")
	for i := range TaxonomyFrameworks {
		if TaxonomyFrameworks[i].Name == name {
			return &TaxonomyFrameworks[i], id
		}
	}
	return nil, id
}

// TaxonomyTitle returns the title of an entry Arena knows, such as "Prompt
// Injection" for owasp-llm:LLM01, or "" for any other entry.
func TaxonomyTitle(entry string) string {
	if f, id := TaxonomyFrameworkOf(entry); f != nil {
		return f.Titles[id]
	}
	return ""
}

// taxonomyEntry is a framework name, a colon and an id, such as
// owasp-llm:LLM01 or atlas:AML.T0051.
var taxonomyEntry = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*:\S+$`)

// atlasID is an ATLAS technique or sub-technique id.
var atlasID = regexp.MustCompile(`^AML\.T\d{4}(\.\d{3})?$`)

// validateTaxonomy checks an objective's taxonomy entries. An entry with a
// {placeholder} needs a dataset, and is checked once each row fills it in
// (see checkDataTaxonomy).
func validateTaxonomy(entries []string, turnIndex int, hasData bool) error {
	seen := map[string]bool{}
	for i, e := range entries {
		where := fmt.Sprintf("turns[%d].objective.taxonomy[%d]", turnIndex, i)
		if seen[e] {
			return fmt.Errorf("%s: %q is listed twice", where, e)
		}
		seen[e] = true
		if len(DataPlaceholders(e)) > 0 {
			if !hasData {
				return fmt.Errorf("%s: %q has a placeholder but the scenario has no data", where, e)
			}
			continue
		}
		if !taxonomyEntry.MatchString(e) {
			return fmt.Errorf("%s: %q must be <framework>:<id>, such as owasp-llm:LLM01", where, e)
		}
		name, id, _ := strings.Cut(e, ":")
		if f, _ := TaxonomyFrameworkOf(e); f != nil && !slices.Contains(f.IDs, id) {
			return fmt.Errorf("%s: %s has no %s; its ids are %s", where, f.Title, id, strings.Join(f.IDs, ", "))
		}
		if name == "atlas" && !atlasID.MatchString(id) {
			return fmt.Errorf("%s: %q is not a MITRE ATLAS technique id, such as atlas:AML.T0051", //nolint:misspell // a name
				where, e)
		}
	}
	return nil
}

// checkDataTaxonomy checks each objective's taxonomy as each dataset row
// fills in its placeholders, so a row value such as "LLM6" is reported
// rather than counted as a category of its own.
func checkDataTaxonomy(s *Scenario, rows []DataRow) error {
	for _, row := range rows {
		filled := s.ForDataRow(row)
		for i := range filled.Turns {
			if o := filled.Turns[i].Objective; o != nil {
				if err := validateTaxonomy(o.Taxonomy, i, false); err != nil {
					return fmt.Errorf("row %s: %w", row.ID, err)
				}
			}
		}
	}
	return nil
}
