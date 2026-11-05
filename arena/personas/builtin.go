// Package personas holds the built-in self-play personas, embedded in the
// binary and copied into a project as ordinary persona YAML for editing.
package personas

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

//go:embed builtin/*.persona.yaml
var builtinFS embed.FS

const (
	builtinDir = "builtin"
	fileSuffix = ".persona.yaml"
	dirPerms   = 0o750
	filePerms  = 0o600
)

// Entry describes one built-in persona.
type Entry struct {
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	Description string `json:"description"`
}

// List returns every built-in persona, sorted by name.
func List() ([]Entry, error) {
	files, err := fs.Glob(builtinFS, builtinDir+"/*"+fileSuffix)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	entries := make([]Entry, 0, len(files))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), fileSuffix)
		p, err := Load(name)
		if err != nil {
			return nil, err
		}
		mode := p.Mode
		if mode == "" {
			mode = arenaconfig.PersonaModeUser
		}
		entries = append(entries, Entry{Name: name, Mode: mode, Description: p.Description})
	}
	return entries, nil
}

// Read returns a built-in persona's YAML exactly as it is embedded.
func Read(name string) ([]byte, error) {
	data, err := builtinFS.ReadFile(builtinDir + "/" + name + fileSuffix)
	if err != nil {
		return nil, fmt.Errorf("no built-in persona %q (see `promptarena personas list`)", name)
	}
	return data, nil
}

// Load parses and validates a built-in persona.
func Load(name string) (*arenaconfig.UserPersonaPack, error) {
	data, err := Read(name)
	if err != nil {
		return nil, err
	}
	return arenaconfig.ParsePersonaYAML(data, name+fileSuffix)
}

// Write copies a built-in persona into dir as <name>.persona.yaml and
// returns the path written. It refuses to replace an existing file unless
// force is set.
func Write(dir, name string, force bool) (string, error) {
	data, err := Read(name)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+fileSuffix)
	if !force {
		if _, err := os.Stat(path); err == nil {
			return "", fmt.Errorf("%s already exists (use --force to replace it)", path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	if err := os.MkdirAll(dir, dirPerms); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, filePerms); err != nil {
		return "", err
	}
	return path, nil
}
