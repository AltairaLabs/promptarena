// Package redteam generates red-team scenarios: it crosses a set of
// objectives with attack strategies and converters and writes the result as
// ordinary, reviewable scenario YAML, so a successful attack becomes a
// regression test.
package redteam

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxObjectiveLineBytes bounds one JSONL line of an objectives file.
const (
	maxObjectiveLineBytes = 1024 * 1024
	initialLineBytes      = 64 * 1024
)

// byteOrderMark is the code point some tools write at the start of a CSV.
const byteOrderMark = 0xFEFF

// Objective is one thing an attack tries to get the agent to do.
type Objective struct {
	// ID names the objective in reports.
	ID string
	// Text says what the attacker is after. It becomes the objective's
	// description, and the persona's goal.
	Text string
	// Prompt is the message a scripted probe sends. Empty means Text.
	Prompt string
	// Category is the source's own category for the objective, if any.
	Category string
	// Taxonomy is one <framework>:<id> entry for the objective, if any.
	Taxonomy string
	// SuccessPattern is text whose appearance in a reply means the attack
	// got through, if the source gives one.
	SuccessPattern string
}

// Column names an objectives file may use, matched case-insensitively. The
// first name of each list is the one generated data files use.
var (
	colID       = []string{"id"}
	colText     = []string{"objective", "goal", "behavior"}
	colPrompt   = []string{"prompt"}
	colCategory = []string{"category"}
	colTaxonomy = []string{"taxonomy"}
	colPattern  = []string{"success_pattern", "win"}
	colBenign   = []string{"benign"}
)

// Objectives holds the objectives read from a source: the ones to attack
// with, and benign look-alikes the agent should answer.
type Objectives struct {
	Harmful []Objective
	Benign  []Objective
}

// ReadObjectivesFile reads a .jsonl or .csv objectives file. Each row needs
// an objective (or goal, or behavior) column; id, prompt, category,
// taxonomy, success_pattern (or win) and benign are optional. A row with a
// true benign column is a benign request the agent should answer, used for
// the over-refusal scenario.
func ReadObjectivesFile(path string) (Objectives, error) {
	f, err := os.Open(path) //nolint:gosec // the objectives file is the operator's own input
	if err != nil {
		return Objectives{}, err
	}
	defer f.Close()

	var records []map[string]string
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".jsonl", ".ndjson":
		records, err = readJSONL(f)
	case ".csv":
		records, err = readCSV(f)
	default:
		return Objectives{}, fmt.Errorf("%s: unsupported extension %q (use .jsonl or .csv)", path, ext)
	}
	if err != nil {
		return Objectives{}, fmt.Errorf("%s: %w", path, err)
	}
	objs, err := objectivesFromRecords(records)
	if err != nil {
		return Objectives{}, fmt.Errorf("%s: %w", path, err)
	}
	return objs, nil
}

func objectivesFromRecords(records []map[string]string) (Objectives, error) {
	var out Objectives
	for i, rec := range records {
		o := Objective{
			ID:             column(rec, colID),
			Text:           column(rec, colText),
			Prompt:         column(rec, colPrompt),
			Category:       column(rec, colCategory),
			Taxonomy:       column(rec, colTaxonomy),
			SuccessPattern: column(rec, colPattern),
		}
		if o.Text == "" {
			return Objectives{}, fmt.Errorf("row %d has no objective (a column named objective, goal or behavior)", i+1)
		}
		if o.ID == "" {
			o.ID = "obj-" + strconv.Itoa(i+1)
		}
		benign, err := truthy(column(rec, colBenign))
		if err != nil {
			return Objectives{}, fmt.Errorf("row %d: benign: %w", i+1, err)
		}
		if benign {
			out.Benign = append(out.Benign, o)
		} else {
			out.Harmful = append(out.Harmful, o)
		}
	}
	if len(out.Harmful)+len(out.Benign) == 0 {
		return Objectives{}, errors.New("no rows")
	}
	return out, nil
}

// column returns the first of names present in rec, ignoring case, trimmed.
func column(rec map[string]string, names []string) string {
	for _, n := range names {
		for k, v := range rec {
			if strings.EqualFold(strings.TrimSpace(k), n) {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func truthy(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "", "false", "no", "0":
		return false, nil
	case "true", "yes", "1":
		return true, nil
	}
	return false, fmt.Errorf("%q is not true or false", s)
}

func readJSONL(r io.Reader) ([]map[string]string, error) {
	var records []map[string]string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, initialLineBytes), maxObjectiveLineBytes)
	line := 0
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
			return nil, fmt.Errorf("line %d: not a JSON object", line)
		}
		rec := make(map[string]string, len(obj))
		for k, v := range obj {
			switch t := v.(type) {
			case nil:
				rec[k] = ""
			case string:
				rec[k] = t
			default:
				b, _ := json.Marshal(t) // a value json.Unmarshal produced always marshals
				rec[k] = string(b)
			}
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

// readCSV reads a CSV file with a header row into records keyed by column.
func readCSV(r io.Reader) ([]map[string]string, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	header[0] = strings.TrimPrefix(header[0], string(rune(byteOrderMark))) // Excel writes a UTF-8 BOM
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
			if i < len(fields) {
				rec[strings.TrimSpace(name)] = fields[i]
			}
		}
		records = append(records, rec)
	}
}
