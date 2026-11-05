package redteam

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Dataset is a public objective set that is fetched on demand, under its own
// license, rather than shipped with Arena.
type Dataset struct {
	// Name is what --dataset takes.
	Name string `json:"name"`
	// Title names the set in generated file headers.
	Title string `json:"title"`
	// Homepage is where the set and its license are published.
	Homepage string `json:"homepage"`
	// License is the set's license, as its publisher states it.
	License string `json:"license"`
	// HarmfulURL is the file of objectives to attack with.
	HarmfulURL string `json:"harmful_url"`
	// BenignURL, when set, is a file of benign look-alikes the agent should
	// answer, for the over-refusal scenario.
	BenignURL string `json:"benign_url,omitempty"`
	// Note says what the generator takes from the set.
	Note string `json:"note"`

	parse func(records []map[string]string, benign bool) (objs []Objective, skipped int)
}

// licenseMIT is the license both public sets are published under.
const licenseMIT = "MIT"

// Datasets are the public sets --dataset can fetch.
var Datasets = []Dataset{
	{
		Name:     "harmbench",
		Title:    "HarmBench text behaviors",
		Homepage: "https://github.com/centerforaisafety/HarmBench",
		License:  licenseMIT,
		HarmfulURL: "https://raw.githubusercontent.com/centerforaisafety/HarmBench/main/" +
			"data/behavior_datasets/harmbench_behaviors_text_all.csv",
		Note:  "standard behaviors only; contextual and copyright behaviors need their context string or hash classifier",
		parse: parseHarmBench,
	},
	{
		Name:       "jbb",
		Title:      "JailbreakBench JBB-Behaviors",
		Homepage:   "https://huggingface.co/datasets/JailbreakBench/JBB-Behaviors",
		License:    licenseMIT,
		HarmfulURL: "https://huggingface.co/datasets/JailbreakBench/JBB-Behaviors/resolve/main/data/harmful-behaviors.csv",
		BenignURL:  "https://huggingface.co/datasets/JailbreakBench/JBB-Behaviors/resolve/main/data/benign-behaviors.csv",
		Note:       "harmful behaviors to attack with; benign behaviors for the over-refusal scenario",
		parse:      parseJBB,
	},
}

// LookupDataset returns the dataset named name.
func LookupDataset(name string) (Dataset, error) {
	names := make([]string, 0, len(Datasets))
	for _, d := range Datasets {
		if d.Name == name {
			return d, nil
		}
		names = append(names, d.Name)
	}
	sort.Strings(names)
	return Dataset{}, fmt.Errorf("unknown dataset %q (known: %s)", name, strings.Join(names, ", "))
}

// harmBenchStandard is the functional category of HarmBench behaviors that
// stand alone as a request.
const harmBenchStandard = "standard"

func parseHarmBench(records []map[string]string, _ bool) ([]Objective, int) {
	var objs []Objective
	skipped := 0
	for _, r := range records {
		if !strings.EqualFold(column(r, []string{"FunctionalCategory"}), harmBenchStandard) {
			skipped++
			continue
		}
		cat := column(r, []string{"SemanticCategory"})
		objs = append(objs, Objective{
			ID:       column(r, []string{"BehaviorID"}),
			Text:     column(r, []string{"Behavior"}),
			Category: cat,
			Taxonomy: categoryEntry("harmbench", cat),
		})
	}
	return objs, skipped
}

func parseJBB(records []map[string]string, benign bool) ([]Objective, int) {
	prefix := "jbb-"
	if benign {
		prefix = "jbb-benign-"
	}
	objs := make([]Objective, 0, len(records))
	for _, r := range records {
		cat := column(r, []string{"Category"})
		objs = append(objs, Objective{
			ID:       prefix + column(r, []string{"Index"}),
			Text:     column(r, []string{"Goal"}),
			Category: cat,
			Taxonomy: categoryEntry("jbb", cat),
		})
	}
	return objs, 0
}

// categoryEntry maps a source category to a taxonomy entry under the
// source's own framework name, such as harmbench:chemical-biological. The
// sets' harm categories have no one-to-one OWASP or ATLAS id, so they are
// kept as the source names them.
func categoryEntry(framework, category string) string {
	slug := Slugify(category)
	if slug == "" {
		slug = "uncategorized"
	}
	return framework + ":" + slug
}

// Slugify lowercases s and turns every run of other characters into a dash.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// Fetcher downloads dataset files into a cache directory.
type Fetcher struct {
	// Client makes the requests. Nil means a client with a 60s timeout.
	Client *http.Client
	// CacheDir holds the downloaded files.
	CacheDir string
	// Refresh downloads a file again even when it is cached.
	Refresh bool
}

// maxDatasetBytes bounds a downloaded dataset file.
const maxDatasetBytes = 64 << 20

const (
	fetchTimeout   = 60 * time.Second
	cacheDirPerms  = 0o750
	cacheFilePerms = 0o600
)

// DefaultCacheDir is where datasets are cached unless --cache-dir says
// otherwise: promptarena/datasets under the user's cache directory.
func DefaultCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "promptarena", "datasets"), nil
}

// Fetch returns the path of url's cached copy, downloading it first unless it
// is cached and Refresh is off.
func (f Fetcher) Fetch(ctx context.Context, url string) (string, error) {
	sum := sha256.Sum256([]byte(url))
	dest := filepath.Join(f.CacheDir, hex.EncodeToString(sum[:8])+"-"+path.Base(url))
	if !f.Refresh {
		if _, err := os.Stat(dest); err == nil {
			return dest, nil
		}
	}
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDatasetBytes+1))
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	if len(data) > maxDatasetBytes {
		return "", fmt.Errorf("fetching %s: larger than %d bytes", url, maxDatasetBytes)
	}
	if err := os.MkdirAll(f.CacheDir, cacheDirPerms); err != nil {
		return "", err
	}
	tmp := dest + ".part"
	if err := os.WriteFile(tmp, data, cacheFilePerms); err != nil {
		return "", err
	}
	return dest, os.Rename(tmp, dest)
}

// LoadDataset reads a dataset's objectives: fetched through f, or, when
// localFile is set, the harmful objectives from that local copy of the
// dataset's file, with no network access and no benign split. It returns
// how many rows it left out, and the source to record in generated files.
func LoadDataset(ctx context.Context, d Dataset, f Fetcher, localFile string) (Objectives, int, Source, error) {
	src := Source{Title: d.Title, Homepage: d.Homepage, License: d.License}
	harmfulPath := localFile
	if localFile == "" {
		p, err := f.Fetch(ctx, d.HarmfulURL)
		if err != nil {
			return Objectives{}, 0, Source{}, err
		}
		harmfulPath = p
		src.URLs = append(src.URLs, d.HarmfulURL)
	} else {
		src.URLs = append(src.URLs, filepath.Base(localFile))
	}
	var objs Objectives
	harmful, skipped, err := parseDatasetFile(harmfulPath, d, false)
	if err != nil {
		return Objectives{}, 0, Source{}, err
	}
	objs.Harmful = harmful
	if d.BenignURL != "" && localFile == "" {
		p, err := f.Fetch(ctx, d.BenignURL)
		if err != nil {
			return Objectives{}, 0, Source{}, err
		}
		benign, _, err := parseDatasetFile(p, d, true)
		if err != nil {
			return Objectives{}, 0, Source{}, err
		}
		objs.Benign = benign
		src.URLs = append(src.URLs, d.BenignURL)
	}
	if len(objs.Harmful) == 0 {
		return Objectives{}, 0, Source{}, fmt.Errorf("%s: no objectives in %s", d.Name, harmfulPath)
	}
	return objs, skipped, src, nil
}

func parseDatasetFile(p string, d Dataset, benign bool) ([]Objective, int, error) {
	data, err := os.ReadFile(p) //nolint:gosec // a cached or operator-provided dataset file
	if err != nil {
		return nil, 0, err
	}
	records, err := readCSV(bytes.NewReader(data))
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", p, err)
	}
	objs, skipped := d.parse(records, benign)
	for i, o := range objs {
		if o.Text == "" || o.ID == "" {
			return nil, 0, fmt.Errorf("%s: row %d has no id or objective; is this the %s file?", p, i+1, d.Name)
		}
	}
	if len(objs) == 0 && skipped == 0 {
		return nil, 0, errors.New(p + ": no rows")
	}
	return objs, skipped, nil
}
