package arenaconfig

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

// dataRowIDField is the column whose value names a row in reports. Rows
// without it are named by their 1-based position in the file.
const dataRowIDField = "id"

// maxDataLineBytes bounds one JSONL line; dataset rows with long documents
// in them outgrow bufio.Scanner's 64KiB default.
const maxDataLineBytes = 16 * 1024 * 1024

// ScenarioData runs a scenario once per row of a dataset file. Each row's
// columns are substituted as {column} into turn content, text parts,
// assertion params and messages, conversation assertions, seed memories and
// variables.
type ScenarioData struct {
	// File is a .jsonl or .csv file, relative to the scenario file (or to
	// the arena config for inline scenario specs).
	File string `json:"file" yaml:"file" jsonschema:"description=Dataset file (.jsonl or .csv) with one row per run; relative to the scenario file"` //nolint:lll
	// Field unwraps each JSONL row to the object under this key, for files
	// that nest the values (e.g. {"item": {...}}). JSONL only.
	Field string `json:"field,omitempty" yaml:"field,omitempty" jsonschema:"description=JSONL only: unwrap each row to the object under this key"` //nolint:lll

	// Rows are the parsed rows, populated by the loader.
	Rows []DataRow `json:"-" yaml:"-"`
}

// DataRow is one row of a scenario dataset.
type DataRow struct {
	// ID names the row in run IDs and reports: the row's "id" column, or its
	// 1-based position in the file.
	ID string
	// Values maps column names to their values, as strings.
	Values map[string]string
}

// validate checks the data block on its own and against the rest of the
// scenario. It runs before the file is read.
func (d *ScenarioData) validate(s *Scenario) error {
	if d.File == "" {
		return errors.New("data.file is required")
	}
	for i := range s.Turns {
		if len(s.Turns[i].Perturbations) > 0 {
			return fmt.Errorf("data and perturbations are mutually exclusive (turn %d defines perturbations)", i+1)
		}
	}
	if d.Field != "" && strings.ToLower(filepath.Ext(d.File)) == ".csv" {
		return errors.New("data.field applies to JSONL files only")
	}
	return nil
}

// loadScenarioData reads the scenario's data file, resolved relative to
// baseFile, and checks that every {placeholder} the scenario uses is a column
// of every row. It is a no-op for scenarios without a data block or whose rows
// are already loaded.
func loadScenarioData(s *Scenario, baseFile string) error {
	if s.Data == nil || s.Data.Rows != nil {
		return nil
	}
	path := s.Data.File
	if !filepath.IsAbs(path) {
		path = config.ResolveFilePath(baseFile, path)
	}
	rows, err := readDataFile(path, s.Data.Field)
	if err != nil {
		return fmt.Errorf("scenario %s: data file %s: %w", s.ID, s.Data.File, err)
	}
	if err := checkDataPlaceholders(s, rows); err != nil {
		return fmt.Errorf("scenario %s: data file %s: %w", s.ID, s.Data.File, err)
	}
	s.Data.Rows = rows
	return nil
}

// readDataFile parses a .jsonl or .csv dataset into rows.
func readDataFile(path, field string) ([]DataRow, error) {
	f, err := os.Open(path) //nosec G304 -- operator-provided dataset path is the loader's purpose
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []map[string]string
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".jsonl", ".ndjson":
		records, err = readJSONLRecords(f, field)
	case ".csv":
		records, err = readCSVRecords(f)
	default:
		return nil, fmt.Errorf("unsupported extension %q (use .jsonl or .csv)", ext)
	}
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("no rows")
	}
	return namedRows(records)
}

func readJSONLRecords(r io.Reader, field string) ([]map[string]string, error) {
	var records []map[string]string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxDataLineBytes)
	line := 0
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		obj, err := decodeJSONObject(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if field != "" {
			inner, ok := obj[field].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("line %d: field %q is not an object", line, field)
			}
			obj = inner
		}
		rec := make(map[string]string, len(obj))
		for k, v := range obj {
			s, err := dataValueString(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: column %q: %w", line, k, err)
			}
			rec[k] = s
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func decodeJSONObject(raw []byte) (map[string]interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // keep numbers as written: 3 stays "3", not "3e+00"
	var obj map[string]interface{}
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("not a JSON object: %w", err)
	}
	if obj == nil {
		return nil, errors.New("not a JSON object")
	}
	if dec.More() {
		return nil, errors.New("more than one JSON value on the line")
	}
	return obj, nil
}

// dataValueString renders a JSON value for substitution: strings as-is,
// null as empty, other scalars as written, and arrays/objects as compact JSON.
func dataValueString(v interface{}) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		return strconv.FormatBool(t), nil
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

func readCSVRecords(r io.Reader) ([]map[string]string, error) {
	reader := csv.NewReader(r)
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	header[0] = strings.TrimPrefix(header[0], "\ufeff") // Excel writes a UTF-8 BOM
	seen := make(map[string]bool, len(header))
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
		switch {
		case header[i] == "":
			return nil, fmt.Errorf("header column %d has no name", i+1)
		case seen[header[i]]:
			return nil, fmt.Errorf("header repeats column %q", header[i])
		}
		seen[header[i]] = true
	}
	var records []map[string]string
	for {
		fields, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return records, nil
		}
		if err != nil {
			return nil, err
		}
		rec := make(map[string]string, len(header))
		for i, name := range header {
			rec[name] = fields[i]
		}
		records = append(records, rec)
	}
}

// namedRows assigns each record its row ID and rejects duplicate IDs, which
// would make two rows indistinguishable in reports.
func namedRows(records []map[string]string) ([]DataRow, error) {
	rows := make([]DataRow, len(records))
	seen := make(map[string]int, len(records))
	for i, rec := range records {
		id := strings.TrimSpace(rec[dataRowIDField])
		if id == "" {
			id = strconv.Itoa(i + 1)
		}
		if prev, dup := seen[id]; dup {
			return nil, fmt.Errorf("rows %d and %d share id %q", prev+1, i+1, id)
		}
		seen[id] = i
		rows[i] = DataRow{ID: id, Values: rec}
	}
	return rows, nil
}

// checkDataPlaceholders fails when the scenario uses a {placeholder} that is
// missing from any row, so a misspelled column is reported rather than sent to
// the model as literal text.
func checkDataPlaceholders(s *Scenario, rows []DataRow) error {
	used := make(map[string]bool)
	s.walkDataTemplates(func(text string) string {
		for _, key := range DataPlaceholders(text) {
			used[key] = true
		}
		return text
	})
	keys := make([]string, 0, len(used))
	for k := range used {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, row := range rows {
		var missing []string
		for _, k := range keys {
			if _, ok := row.Values[k]; !ok {
				missing = append(missing, "{"+k+"}")
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("row %s has no column for %s", row.ID, strings.Join(missing, ", "))
		}
	}
	return nil
}

// ForDataRow returns a copy of the scenario with the row's values
// substituted. The original scenario is not modified.
func (s *Scenario) ForDataRow(row DataRow) *Scenario {
	out := *s
	out.Turns = make([]TurnDefinition, len(s.Turns))
	for i := range s.Turns {
		out.Turns[i] = s.Turns[i]
		out.Turns[i].Parts = append([]TurnContentPart(nil), s.Turns[i].Parts...)
		out.Turns[i].Assertions = append([]AssertionConfig(nil), s.Turns[i].Assertions...)
	}
	out.ConversationAssertions = append([]AssertionConfig(nil), s.ConversationAssertions...)
	out.SeedMemories = append([]SeedMemoryEntry(nil), s.SeedMemories...)
	out.Variables = maps.Clone(s.Variables)
	out.walkDataTemplates(func(text string) string {
		return SubstituteDataPlaceholders(text, row.Values)
	})
	return &out
}

// walkDataTemplates calls fn on every string that takes row substitutions and
// stores what fn returns. Assertion params are rebuilt rather than edited in
// place, so a copy made by ForDataRow never writes into the original's maps.
func (s *Scenario) walkDataTemplates(fn func(string) string) {
	for i := range s.Turns {
		turn := &s.Turns[i]
		turn.Content = fn(turn.Content)
		for j := range turn.Parts {
			if turn.Parts[j].Type == "text" {
				turn.Parts[j].Text = fn(turn.Parts[j].Text)
			}
		}
		walkAssertionTemplates(turn.Assertions, fn)
	}
	walkAssertionTemplates(s.ConversationAssertions, fn)
	for i := range s.SeedMemories {
		s.SeedMemories[i].Content = fn(s.SeedMemories[i].Content)
	}
	for k, v := range s.Variables {
		s.Variables[k] = fn(v)
	}
}

func walkAssertionTemplates(assertions []AssertionConfig, fn func(string) string) {
	for i := range assertions {
		assertions[i].Message = fn(assertions[i].Message)
		if assertions[i].Params != nil {
			assertions[i].Params, _ = mapTemplateValue(assertions[i].Params, fn).(map[string]interface{})
		}
	}
}

// mapTemplateValue returns v with fn applied to every string inside it.
func mapTemplateValue(v interface{}, fn func(string) string) interface{} {
	switch t := v.(type) {
	case string:
		return fn(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i := range t {
			out[i] = mapTemplateValue(t[i], fn)
		}
		return out
	case []string:
		out := make([]string, len(t))
		for i := range t {
			out[i] = fn(t[i])
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = mapTemplateValue(val, fn)
		}
		return out
	default:
		return v
	}
}

// DataPlaceholders returns the keys of the {key} placeholders in text, in
// order of appearance. Keys start with a letter or underscore and continue
// with letters, digits, '_', '-' or '.'. Double-braced {{template}} variables
// are not placeholders.
func DataPlaceholders(text string) []string {
	var keys []string
	scanPlaceholders(text, func(key string) (string, bool) {
		keys = append(keys, key)
		return "", false
	})
	return keys
}

// SubstituteDataPlaceholders replaces each {key} placeholder in text whose key
// is in values. Substituted values are not scanned again, and double-braced
// {{template}} variables are left for the prompt template engine.
func SubstituteDataPlaceholders(text string, values map[string]string) string {
	if !strings.Contains(text, "{") {
		return text
	}
	return scanPlaceholders(text, func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	})
}

// scanPlaceholders walks text once, calling replace for every {key}
// placeholder; where replace reports true the placeholder is replaced.
func scanPlaceholders(text string, replace func(key string) (string, bool)) string {
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); {
		if text[i] != '{' {
			b.WriteByte(text[i])
			i++
			continue
		}
		// A run of two or more braces opens a {{template}}: copy it untouched.
		if i+1 < len(text) && text[i+1] == '{' {
			j := i
			for j < len(text) && text[j] == '{' {
				j++
			}
			b.WriteString(text[i:j])
			i = j
			continue
		}
		end := placeholderEnd(text, i+1)
		if end < 0 {
			b.WriteByte('{')
			i++
			continue
		}
		key := text[i+1 : end]
		if v, ok := replace(key); ok {
			b.WriteString(v)
		} else {
			b.WriteString(text[i : end+1])
		}
		i = end + 1
	}
	return b.String()
}

// placeholderEnd returns the index of the '}' closing a placeholder key that
// starts at start, or -1 if no valid key starts there.
func placeholderEnd(text string, start int) int {
	j := start
	for j < len(text) && isPlaceholderChar(text[j], j == start) {
		j++
	}
	if j == start || j >= len(text) || text[j] != '}' {
		return -1
	}
	if j+1 < len(text) && text[j+1] == '}' {
		return -1 // "{key}}" is template syntax, not a placeholder
	}
	return j
}

func isPlaceholderChar(c byte, first bool) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		return true
	case first:
		return false
	case c >= '0' && c <= '9', c == '-', c == '.':
		return true
	}
	return false
}
