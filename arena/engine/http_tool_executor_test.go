package engine

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// httpToolDescriptor builds a live-mode HTTP tool pointing at url.
func httpToolDescriptor(url string) *tools.ToolDescriptor {
	return &tools.ToolDescriptor{
		Name:      "fetch",
		Mode:      "live",
		TimeoutMs: 5000,
		HTTPConfig: &tools.HTTPConfig{
			URL:    url,
			Method: http.MethodGet,
		},
	}
}

// bodyServer returns a server answering every request with n bytes of JSON.
func bodyServer(t *testing.T, n int) *httptest.Server {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"data": strings.Repeat("x", n)})
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// One run exhausting its budget must not affect another. Before this, arena
// shared a single cumulative counter across the whole matrix, so a late run
// could fail because of traffic from earlier, unrelated runs.
func TestHTTPToolExecutor_BudgetIsPerRun(t *testing.T) {
	srv := bodyServer(t, 512)
	desc := httpToolDescriptor(srv.URL)

	// A budget small enough that one response exhausts it.
	exec := newHTTPToolExecutor(64)

	ctxA := withRunID(t.Context(), "run-a")
	_, err := exec.Execute(ctxA, desc, json.RawMessage(`{}`))
	require.NoError(t, err, "the first call in a run is always allowed")

	// run-a is now over budget.
	_, err = exec.Execute(ctxA, desc, json.RawMessage(`{}`))
	require.Error(t, err)
	assert.ErrorIs(t, err, tools.ErrAggregateResponseSizeExceeded)

	// run-b is untouched by run-a's spending.
	ctxB := withRunID(t.Context(), "run-b")
	_, err = exec.Execute(ctxB, desc, json.RawMessage(`{}`))
	assert.NoError(t, err, "run B must have its own budget")
}

func TestHTTPToolExecutor_AccumulatesWithinARun(t *testing.T) {
	srv := bodyServer(t, 100)
	desc := httpToolDescriptor(srv.URL)

	exec := newHTTPToolExecutor(0) // unlimited, so we can watch the total grow
	ctx := withRunID(t.Context(), "run-a")

	_, err := exec.Execute(ctx, desc, json.RawMessage(`{}`))
	require.NoError(t, err)
	first := exec.usage["run-a"]
	require.Positive(t, first)

	_, err = exec.Execute(ctx, desc, json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Greater(t, exec.usage["run-a"], first, "a second response must add to the run's total")
}

func TestHTTPToolExecutor_ZeroBudgetIsUnlimited(t *testing.T) {
	srv := bodyServer(t, 256)
	desc := httpToolDescriptor(srv.URL)

	exec := newHTTPToolExecutor(0)
	ctx := withRunID(t.Context(), "run-a")

	for range 5 {
		_, err := exec.Execute(ctx, desc, json.RawMessage(`{}`))
		require.NoError(t, err)
	}
}

// Calls with no run identity still meter, sharing one bucket, so behavior
// outside a run matches what the wrapped executor used to do.
func TestHTTPToolExecutor_NoRunIdentitySharesOneBucket(t *testing.T) {
	srv := bodyServer(t, 512)
	desc := httpToolDescriptor(srv.URL)

	exec := newHTTPToolExecutor(64)

	_, err := exec.Execute(t.Context(), desc, json.RawMessage(`{}`))
	require.NoError(t, err)

	_, err = exec.Execute(t.Context(), desc, json.RawMessage(`{}`))
	assert.True(t, errors.Is(err, tools.ErrAggregateResponseSizeExceeded))
}

// The registry prefers ExecuteMultimodal when an executor offers it. The
// wrapper must forward it, or binary HTTP responses silently lose their content
// parts.
func TestHTTPToolExecutor_ImplementsMultimodal(t *testing.T) {
	var _ tools.MultimodalExecutor = newHTTPToolExecutor(0)

	srv := bodyServer(t, 32)
	exec := newHTTPToolExecutor(0)
	ctx := withRunID(t.Context(), "run-a")

	result, _, err := exec.ExecuteMultimodal(ctx, httpToolDescriptor(srv.URL), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.Positive(t, exec.usage["run-a"], "multimodal responses must be metered too")
}

func TestHTTPToolExecutor_NameMatchesWrappedExecutor(t *testing.T) {
	assert.Equal(t, tools.NewHTTPExecutor().Name(), newHTTPToolExecutor(0).Name())
}
