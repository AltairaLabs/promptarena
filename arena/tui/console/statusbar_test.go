package console

import (
	"bytes"
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

func TestLiveIsSilentWhenNotTTY(t *testing.T) {
	sb := NewStatusBar(4, false)
	sb.Advance(nil)

	var buf bytes.Buffer
	sb.Live(&buf)
	if buf.Len() != 0 {
		t.Errorf("Live on a non-TTY wrote %q, want nothing", buf.String())
	}
}

func TestLiveRepaintsInPlaceOnTTY(t *testing.T) {
	sb := NewStatusBar(4, true)
	sb.Advance(nil)

	var buf bytes.Buffer
	sb.Live(&buf)
	out := buf.String()
	if !strings.HasPrefix(out, "\r") {
		t.Errorf("Live on a TTY = %q, want a leading carriage return", out)
	}
	if !strings.Contains(plain(out), "1/4") {
		t.Errorf("Live on a TTY = %q, want the progress count", plain(out))
	}
}

func TestFinishWritesFinalLineWithNewline(t *testing.T) {
	sb := NewStatusBar(2, false)
	sb.Advance(nil)
	sb.Advance(nil)

	var buf bytes.Buffer
	sb.Finish(&buf)
	out := plain(buf.String())
	if !strings.Contains(out, "Done") || !strings.Contains(out, "2/2") {
		t.Errorf("Finish() = %q, want the completed summary", out)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("Finish() = %q, want a trailing newline", buf.String())
	}
}

var errFail = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "boom" }
