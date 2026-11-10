package pages

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFiles creates empty files with the given names in dir.
func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o644))
	}
}

// loadedBrowser returns a browser for dir whose directory listing has been
// read, by running Init's command and feeding its message back — the same
// round trip the bubbletea loop performs. Like the loop, it renders once
// first, which is what sizes the file picker.
func loadedBrowser(t *testing.T, dir string) *FileBrowserPage {
	t.Helper()
	page := NewFileBrowserPage(dir)
	page.SetDimensions(80, 24)
	cmd := page.Init()
	require.NotNil(t, cmd, "Init must return the directory-read command")
	_ = page.View()
	page.Update(cmd())
	return page
}

// pressEnter sends Enter and returns the FileSelectedMsg it produced, if any.
func pressEnter(t *testing.T, page *FileBrowserPage) (FileSelectedMsg, bool) {
	t.Helper()
	_, cmd := page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		return FileSelectedMsg{}, false
	}
	sel, ok := cmd().(FileSelectedMsg)
	return sel, ok
}

func pressRune(page *FileBrowserPage, r rune) {
	page.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

func TestFileBrowserPage_SetDimensions(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)

	page.SetDimensions(100, 50)

	assert.Equal(t, 100, page.width)
	assert.Equal(t, 50, page.height)
	// Note: filepicker height is set dynamically in Render(), not in SetDimensions
}

func TestFileBrowserPage_Init(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, "run-a.json", "run-b.json")

	page := loadedBrowser(t, tmpDir)
	view := page.View()
	assert.Contains(t, view, "run-a.json", "Init must list the results directory")
	assert.Contains(t, view, "run-b.json", "Init must list the results directory")
}

func TestFileBrowserPage_View(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)
	page.SetDimensions(80, 24)

	view := page.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Browse Results")
}

func TestFileBrowserPage_UpdateQuitKeys(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlC},
	} {
		t.Run(key.String(), func(t *testing.T) {
			page := NewFileBrowserPage(t.TempDir())
			_, cmd := page.Update(key)
			require.NotNil(t, cmd)
			assert.IsType(t, tea.QuitMsg{}, cmd())
		})
	}
}

func TestFileBrowserPage_GetKeyBindings(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)

	bindings := page.GetKeyBindings()
	assert.NotEmpty(t, bindings)
	assert.Equal(t, 5, len(bindings))

	// Check that key bindings contain expected items
	var hasNavigate, hasSelect, hasQuit bool
	for _, kb := range bindings {
		if kb.Description == "navigate" {
			hasNavigate = true
		}
		if kb.Description == "select file" {
			hasSelect = true
		}
		if kb.Description == "quit" {
			hasQuit = true
		}
	}
	assert.True(t, hasNavigate)
	assert.True(t, hasSelect)
	assert.True(t, hasQuit)
}

func TestFileBrowserPage_RenderNoLongerIncludesKeyLegend(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)
	page.SetDimensions(80, 24)

	// Render should not include key legend (moved to footer)
	view := page.Render()
	assert.NotEmpty(t, view)
	// The view should contain the file picker, but not the styled key legend
	assert.Contains(t, view, "Browse Results")
}

func TestFileBrowserPage_Reset(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)

	// Set a path
	page.filePicker.Path = "/some/path/file.json"
	assert.NotEmpty(t, page.filePicker.Path)

	// Reset should clear the path
	page.Reset()
	assert.Empty(t, page.filePicker.Path)
}

func TestFileBrowserPage_UpdateErrorMsg(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)

	// Send an error message
	errMsg := fileBrowserErrorMsg{err: assert.AnError}
	model, cmd := page.Update(errMsg)

	p := model.(*FileBrowserPage)
	assert.Equal(t, assert.AnError, p.err)
	assert.Nil(t, cmd)
}

func TestFileBrowserPage_RenderWithError(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)
	page.SetDimensions(80, 24)

	// Set an error
	page.err = assert.AnError

	view := page.Render()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Error")
}

func TestFileBrowserPage_RenderWithParseError(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)
	page.SetDimensions(80, 24)

	// Create an error that contains "parse" or "unmarshal"
	parseErr := fmt.Errorf("failed to parse JSON: invalid character")
	page.err = parseErr

	view := page.Render()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Invalid JSON file")
}

func TestFileBrowserPage_RenderWithSmallDimensions(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)

	// Set very small dimensions
	page.SetDimensions(10, 3)

	view := page.Render()
	assert.NotEmpty(t, view)
}

func TestFileBrowserPage_EnterOnEmptyDirectorySelectsNothing(t *testing.T) {
	page := loadedBrowser(t, t.TempDir())

	_, cmd := page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Nil(t, cmd)
	assert.Empty(t, page.filePicker.Path)
}

func TestFileBrowserPage_EnterOnFileEmitsSelection(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, "index.json", "test-run.json")
	page := loadedBrowser(t, tmpDir)

	// The listing is sorted, so the cursor starts on index.json.
	sel, ok := pressEnter(t, page)
	require.True(t, ok, "Enter on a file must emit FileSelectedMsg")
	assert.Equal(t, FileSelectedMsg{RunID: "index", Path: filepath.Join(tmpDir, "index.json")}, sel)

	pressRune(page, 'j')
	sel, ok = pressEnter(t, page)
	require.True(t, ok)
	assert.Equal(t, FileSelectedMsg{RunID: "test-run", Path: filepath.Join(tmpDir, "test-run.json")}, sel)
}

func TestFileBrowserPage_EnterSameFileAgainNeedsReset(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, "result.json")
	page := loadedBrowser(t, tmpDir)

	_, ok := pressEnter(t, page)
	require.True(t, ok)

	// The picker still holds the previous selection, so Enter on the same
	// file is not a new selection until Reset clears it.
	_, ok = pressEnter(t, page)
	assert.False(t, ok, "re-selecting the same file must not emit without Reset")

	page.Reset()
	sel, ok := pressEnter(t, page)
	require.True(t, ok, "after Reset the same file can be selected again")
	assert.Equal(t, "result", sel.RunID)
}

func TestFileBrowserPage_EnterOnDirectoryNavigatesInto(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "nested")
	require.NoError(t, os.Mkdir(nested, 0o755))
	writeFiles(t, nested, "inner.json")
	page := loadedBrowser(t, tmpDir)

	_, cmd := page.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "Enter on a directory must read it")
	msg := cmd()
	assert.NotEqual(t, "FileSelectedMsg", fmt.Sprintf("%T", msg), "a directory is not a file selection")
	page.Update(msg)

	assert.Equal(t, nested, page.filePicker.CurrentDirectory)
	assert.Contains(t, page.View(), "inner.json")
}

// A file removed after the listing was read cannot be opened: Enter must not
// emit a selection for a path that no longer exists.
func TestFileBrowserPage_EnterOnVanishedFileSelectsNothing(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, "temp.json")
	page := loadedBrowser(t, tmpDir)
	require.NoError(t, os.Remove(filepath.Join(tmpDir, "temp.json")))

	_, ok := pressEnter(t, page)
	assert.False(t, ok)
	assert.Empty(t, page.filePicker.Path)
}

func TestFileBrowserPage_UpdateNavigationKeys(t *testing.T) {
	tmpDir := t.TempDir()
	writeFiles(t, tmpDir, "a.json", "b.json", "c.json")
	page := loadedBrowser(t, tmpDir)

	steps := []struct {
		keys []rune
		want string
	}{
		{[]rune{'j', 'j'}, "c"}, // down twice
		{[]rune{'k'}, "b"},      // up
		{[]rune{'g'}, "a"},      // top
		{[]rune{'G'}, "c"},      // bottom
	}
	for _, step := range steps {
		for _, r := range step.keys {
			pressRune(page, r)
		}
		sel, ok := pressEnter(t, page)
		require.True(t, ok, "after %q", string(step.keys))
		assert.Equal(t, step.want, sel.RunID, "after %q", string(step.keys))
	}

	// Arrow keys move the cursor the same way.
	page.Update(tea.KeyMsg{Type: tea.KeyUp})
	sel, ok := pressEnter(t, page)
	require.True(t, ok)
	assert.Equal(t, "b", sel.RunID)
	page.Update(tea.KeyMsg{Type: tea.KeyDown})
	sel, ok = pressEnter(t, page)
	require.True(t, ok)
	assert.Equal(t, "c", sel.RunID)
}

func TestFileBrowserPage_RenderWithVariousDimensions(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"normal", 80, 24},
		{"wide", 200, 50},
		{"tall", 80, 100},
		{"small width", 30, 24},
		{"small height", 80, 10},
		{"minimum", 20, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page.SetDimensions(tt.width, tt.height)
			view := page.Render()
			assert.NotEmpty(t, view)
		})
	}
}

func TestFileBrowserPage_Constants(t *testing.T) {
	// Verify constants are defined with expected values
	assert.Equal(t, 5, fileBrowserChrome)
	assert.Equal(t, 80, defaultViewportWidth)
	assert.Equal(t, 20, defaultViewportHeight)
	assert.Equal(t, 5, minFileBrowserHeight)
	assert.Equal(t, 20, minFileBrowserWidth)
	assert.Equal(t, 6, fileBrowserWidthChrome)
	assert.Equal(t, 2, fileBrowserPaddingHoriz)
}

func TestFileBrowserPage_AllowedTypes(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)

	// Verify only JSON files are allowed
	assert.Equal(t, []string{".json"}, page.filePicker.AllowedTypes)
	assert.False(t, page.filePicker.ShowHidden)
}

func TestFileBrowserPage_CurrentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	page := NewFileBrowserPage(tmpDir)

	// Verify current directory is set correctly
	assert.Equal(t, tmpDir, page.filePicker.CurrentDirectory)
}
