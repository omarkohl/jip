// Package config loads jip's persistent preferences from TOML config files.
//
// Two locations are consulted, in order:
//  1. Global: <user config dir>/jip/config.toml (e.g. ~/.config/jip/config.toml)
//  2. Repo:   .jip.toml in the repository root
//
// Repo values override global values. CLI flags override both (enforced by
// the caller, which only applies config to flags not set on the command line).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Dir overrides the global config directory for testing.
// If empty, os.UserConfigDir() is used.
var Dir string

// GlobalPath returns the path of the global config file.
func GlobalPath() (string, error) {
	dir := Dir
	if dir == "" {
		var err error
		dir, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "jip", "config.toml"), nil
}

// Load reads the global and repo config files and returns a merged key→value
// map with repo values taking precedence. Values are normalized to strings
// ready to be applied to command-line flags (arrays are joined with commas).
// Missing files are not an error; repoRoot may be empty to skip the repo file.
func Load(repoRoot string) (map[string]string, error) {
	merged := make(map[string]string)

	// The global config is an optional convenience: if its location can't be
	// determined (e.g. os.UserConfigDir() fails because $HOME is unset),
	// proceed as if there were no global config rather than aborting.
	if globalPath, err := GlobalPath(); err == nil {
		global, err := loadFile(globalPath)
		if err != nil {
			return nil, err
		}
		for k, v := range global {
			merged[k] = v
		}
	}

	if repoRoot != "" {
		repo, err := loadFile(filepath.Join(repoRoot, ".jip.toml"))
		if err != nil {
			return nil, err
		}
		for k, v := range repo {
			merged[k] = v
		}
	}
	return merged, nil
}

// loadFile parses a single TOML config file into flag-ready string values.
// A missing file yields an empty map.
func loadFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	cfg := make(map[string]string, len(raw))
	for key, val := range raw {
		s, err := stringify(val)
		if err != nil {
			return nil, fmt.Errorf("config %s: key %q: %w", path, key, err)
		}
		cfg[key] = s
	}
	return cfg, nil
}

// stringify converts a TOML value to a flag-ready string.
func stringify(val any) (string, error) {
	switch v := val.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case []any:
		parts := make([]string, len(v))
		for i, e := range v {
			s, ok := e.(string)
			if !ok {
				return "", fmt.Errorf("array elements must be strings, got %T", e)
			}
			parts[i] = s
		}
		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("unsupported value type %T (use a string, boolean, integer, or array of strings)", val)
	}
}
