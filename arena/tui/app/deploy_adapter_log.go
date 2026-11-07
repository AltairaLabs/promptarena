package app

import (
	"bytes"
	"strings"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
)

// adapterLogWriter turns a deploy adapter's stderr into log lines. The TUI
// installs it with flow.SetAdapterStderr so adapter output reaches the log
// interceptor (and the log file in verbose mode) instead of being written
// straight to the terminal over the alt-screen.
type adapterLogWriter struct {
	mu      sync.Mutex
	partial []byte
	log     func(line string)
}

// newAdapterLogWriter returns a writer that logs each complete line at info
// level, prefixed so it reads as adapter output.
func newAdapterLogWriter() *adapterLogWriter {
	return &adapterLogWriter{log: func(line string) { logger.Info("deploy adapter: " + line) }}
}

// Write buffers p and logs every complete, non-blank line.
func (w *adapterLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.partial = append(w.partial, p...)
	for {
		i := bytes.IndexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.partial[:i]), "\r")
		w.partial = w.partial[i+1:]
		if strings.TrimSpace(line) != "" {
			w.log(line)
		}
	}
	return len(p), nil
}
