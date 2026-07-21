package console

import (
	"regexp"
	"strings"
	"testing"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

func TestStatusBarShowsProgressCount(t *testing.T) {
	sb := NewStatusBar(12, false)
	for i := 0; i < 8; i++ {
		sb.Advance(nil)
	}

	got := plain(sb.Render())
	if !strings.Contains(got, "8/12") {
		t.Errorf("Render() = %q, want it to contain 8/12", got)
	}
}

func TestStatusBarFillIsProportional(t *testing.T) {
	sb := NewStatusBar(10, false)
	for i := 0; i < 5; i++ {
		sb.Advance(nil)
	}

	got := plain(sb.Render())
	filled := strings.Count(got, "█")
	empty := strings.Count(got, "░")

	// Half done → half the bar filled.
	if filled == 0 || filled != empty {
		t.Errorf("at 5/10 expected an equal fill/empty split, got filled=%d empty=%d in %q", filled, empty, got)
	}
}

func TestStatusBarReportsFailures(t *testing.T) {
	sb := NewStatusBar(4, false)
	sb.Advance(nil)
	sb.Advance(errFail)
	sb.Advance(errFail)

	got := plain(sb.Render())
	if !strings.Contains(got, "2 failed") {
		t.Errorf("Render() = %q, want it to report 2 failed", got)
	}
}

func TestStatusBarOmitsFailureSuffixWhenClean(t *testing.T) {
	sb := NewStatusBar(4, false)
	sb.Advance(nil)
	sb.Advance(nil)

	got := plain(sb.Render())
	if strings.Contains(got, "failed") {
		t.Errorf("Render() = %q, want no failure suffix when nothing failed", got)
	}
}

func TestStatusBarLabelFlipsWhenComplete(t *testing.T) {
	sb := NewStatusBar(2, false)
	if strings.Contains(plain(sb.Render()), "Done") {
		t.Fatal("fresh bar should not read Done")
	}
	sb.Advance(nil)
	sb.Advance(nil)
	if !strings.Contains(plain(sb.Render()), "Done") {
		t.Errorf("after all runs complete, Render() = %q, want it to read Done", plain(sb.Render()))
	}
}

var errFail = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "boom" }
