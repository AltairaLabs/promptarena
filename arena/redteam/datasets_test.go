package redteam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures stand in for the public sets with their real columns and
// placeholder text: no test reads the network or the sets themselves.
const (
	harmBenchCSV = "Behavior,FunctionalCategory,SemanticCategory,Tags,ContextString,BehaviorID\n" +
		"Placeholder behavior one,standard,chemical_biological,,,hb_one\n" +
		"Placeholder behavior two,contextual,illegal,context,\"some, context\",hb_two\n" +
		"Placeholder behavior three,standard,harassment_bullying,,,hb_three\n" +
		"Placeholder lyrics,copyright,copyright,hash_check,,hb_four\n"
	jbbHarmfulCSV = "Index,Goal,Target,Behavior,Category,Source\n" +
		"1,Placeholder goal one,Sure,Defamation,Harassment/Discrimination,Original\n" +
		"2,Placeholder goal two,Sure,Malware,Malware/Hacking,Original\n"
	jbbBenignCSV = "Index,Goal,Target,Behavior,Category,Source\n" +
		"1,Placeholder benign one,Sure,History,Harassment/Discrimination,Original\n"
)

// datasetServer serves the fixtures and counts requests.
func datasetServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	files := map[string]string{
		"/harmbench.csv": harmBenchCSV, "/jbb-harmful.csv": jbbHarmfulCSV, "/jbb-benign.csv": jbbBenignCSV,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func pointAt(t *testing.T, name, harmful, benign string) Dataset {
	t.Helper()
	d, err := LookupDataset(name)
	require.NoError(t, err)
	d.HarmfulURL = harmful
	d.BenignURL = benign
	return d
}

func TestLookupDataset(t *testing.T) {
	d, err := LookupDataset("jbb")
	require.NoError(t, err)
	assert.Equal(t, "MIT", d.License)
	assert.NotEmpty(t, d.BenignURL)

	_, err = LookupDataset("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "harmbench, jbb")
}

func TestLoadDataset_HarmBenchKeepsStandardBehaviors(t *testing.T) {
	srv, _ := datasetServer(t)
	d := pointAt(t, "harmbench", srv.URL+"/harmbench.csv", "")
	objs, skipped, src, err := LoadDataset(context.Background(), d, Fetcher{CacheDir: t.TempDir()}, "")
	require.NoError(t, err)
	assert.Equal(t, 2, skipped)
	require.Len(t, objs.Harmful, 2)
	assert.Empty(t, objs.Benign)
	assert.Equal(t, Objective{ID: "hb_one", Text: "Placeholder behavior one", Category: "chemical_biological",
		Taxonomy: "harmbench:chemical-biological"}, objs.Harmful[0])
	assert.Equal(t, []string{srv.URL + "/harmbench.csv"}, src.URLs)
	assert.Equal(t, "MIT", src.License)
}

func TestLoadDataset_JBBWithBenignSplit(t *testing.T) {
	srv, _ := datasetServer(t)
	d := pointAt(t, "jbb", srv.URL+"/jbb-harmful.csv", srv.URL+"/jbb-benign.csv")
	objs, skipped, src, err := LoadDataset(context.Background(), d, Fetcher{CacheDir: t.TempDir()}, "")
	require.NoError(t, err)
	assert.Zero(t, skipped)
	require.Len(t, objs.Harmful, 2)
	assert.Equal(t, "jbb-1", objs.Harmful[0].ID)
	assert.Equal(t, "jbb:harassment-discrimination", objs.Harmful[0].Taxonomy)
	assert.Equal(t, "jbb:malware-hacking", objs.Harmful[1].Taxonomy)
	require.Len(t, objs.Benign, 1)
	assert.Equal(t, "jbb-benign-1", objs.Benign[0].ID)
	assert.Len(t, src.URLs, 2)
}

func TestLoadDataset_LocalFileNeedsNoNetwork(t *testing.T) {
	local := writeTemp(t, "harmful-behaviors.csv", jbbHarmfulCSV)
	d := pointAt(t, "jbb", "http://127.0.0.1:1/unreachable.csv", "http://127.0.0.1:1/unreachable.csv")
	objs, _, src, err := LoadDataset(context.Background(), d, Fetcher{CacheDir: t.TempDir()}, local)
	require.NoError(t, err)
	assert.Len(t, objs.Harmful, 2)
	assert.Empty(t, objs.Benign)
	assert.Equal(t, []string{"harmful-behaviors.csv"}, src.URLs)
}

func TestLoadDataset_Errors(t *testing.T) {
	srv, _ := datasetServer(t)
	ctx := context.Background()

	_, _, _, err := LoadDataset(ctx, pointAt(t, "jbb", srv.URL+"/missing.csv", ""), Fetcher{CacheDir: t.TempDir()}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")

	_, _, _, err = LoadDataset(ctx, pointAt(t, "jbb", srv.URL+"/jbb-harmful.csv", srv.URL+"/missing.csv"),
		Fetcher{CacheDir: t.TempDir()}, "")
	require.Error(t, err)

	// The wrong file for the dataset names the problem.
	wrong := writeTemp(t, "wrong.csv", "a,b\n1,2\n")
	_, _, _, err = LoadDataset(ctx, pointAt(t, "jbb", "", ""), Fetcher{}, wrong)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is this the jbb file")

	// Only rows the generator leaves out.
	onlyContextual := writeTemp(t, "hb.csv", "Behavior,FunctionalCategory,SemanticCategory,BehaviorID\nx,contextual,illegal,x\n")
	_, _, _, err = LoadDataset(ctx, pointAt(t, "harmbench", "", ""), Fetcher{}, onlyContextual)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no objectives")

	empty := writeTemp(t, "empty.csv", "")
	_, _, _, err = LoadDataset(ctx, pointAt(t, "jbb", "", ""), Fetcher{}, empty)
	require.Error(t, err)

	_, _, _, err = LoadDataset(ctx, pointAt(t, "jbb", "", ""), Fetcher{}, filepath.Join(t.TempDir(), "none.csv"))
	require.Error(t, err)

	bad := writeTemp(t, "bad.csv", "Index,Goal\n\"unterminated\n")
	_, _, _, err = LoadDataset(ctx, pointAt(t, "jbb", "", ""), Fetcher{}, bad)
	require.Error(t, err)
}

func TestFetch_CachesAndRefreshes(t *testing.T) {
	srv, hits := datasetServer(t)
	cache := filepath.Join(t.TempDir(), "cache")
	f := Fetcher{CacheDir: cache, Client: srv.Client()}
	url := srv.URL + "/jbb-harmful.csv"

	p1, err := f.Fetch(context.Background(), url)
	require.NoError(t, err)
	p2, err := f.Fetch(context.Background(), url)
	require.NoError(t, err)
	assert.Equal(t, p1, p2)
	assert.Equal(t, int32(1), hits.Load(), "a cached file is not fetched again")
	got, err := os.ReadFile(p1)
	require.NoError(t, err)
	assert.Equal(t, jbbHarmfulCSV, string(got))

	f.Refresh = true
	_, err = f.Fetch(context.Background(), url)
	require.NoError(t, err)
	assert.Equal(t, int32(2), hits.Load())
}

func TestFetch_Errors(t *testing.T) {
	ctx := context.Background()
	_, err := Fetcher{CacheDir: t.TempDir()}.Fetch(ctx, "http://127.0.0.1:1/x.csv")
	require.Error(t, err)

	_, err = Fetcher{CacheDir: t.TempDir()}.Fetch(ctx, "::not a url")
	require.Error(t, err)

	// The cache directory can't be created under a file.
	srv, _ := datasetServer(t)
	blocker := writeTemp(t, "file", "x")
	_, err = Fetcher{CacheDir: filepath.Join(blocker, "cache")}.Fetch(ctx, srv.URL+"/jbb-harmful.csv")
	require.Error(t, err)
}

func TestSlugifyAndCategoryEntry(t *testing.T) {
	assert.Equal(t, "harassment-discrimination", Slugify("Harassment/Discrimination"))
	assert.Equal(t, "a-b", Slugify("  A  b!! "))
	assert.Equal(t, "x:uncategorized", categoryEntry("x", "///"))
}

func TestDefaultCacheDir(t *testing.T) {
	dir, err := DefaultCacheDir()
	require.NoError(t, err)
	assert.Equal(t, "datasets", filepath.Base(dir))
}
