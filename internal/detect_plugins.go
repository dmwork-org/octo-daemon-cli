package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// openclawPluginsListJSON is the shape returned by `openclaw plugins list --json`.
// We only use id/version/enabled; other fields parsed for potential future use.
type openclawPluginJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Origin  string `json:"origin"`
}

type openclawPluginsListJSON struct {
	Plugins []openclawPluginJSON `json:"plugins"`
}

// parseOpenclawPluginsJSON parses output of `openclaw plugins list --json`.
// Returns only enabled plugins as PluginInfo{Name: id, Version: version}.
// Name field on the wire is the npm/id string (not the human display name) because
// the server and frontend match plugins by name == "openclaw-channel-dmwork".
func parseOpenclawPluginsJSON(data []byte) ([]PluginInfo, error) {
	obj := extractJSONObject(data)
	if obj == nil {
		return nil, fmt.Errorf("no JSON object found in output")
	}
	var raw openclawPluginsListJSON
	if err := json.Unmarshal(obj, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal plugins list: %w", err)
	}
	var out []PluginInfo
	for _, p := range raw.Plugins {
		if !p.Enabled {
			continue
		}
		if p.ID == "" || p.Version == "" {
			continue
		}
		out = append(out, PluginInfo{Name: p.ID, Version: p.Version})
	}
	return out, nil
}

// extractJSONObject extracts a top-level JSON object from output that may have
// prefix/suffix noise. Most of the time openclaw plugins list --json emits pure
// JSON on stdout (warnings go to stderr), but we guard against future changes.
func extractJSONObject(data []byte) []byte {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var probe map[string]json.RawMessage
		if json.Unmarshal(trimmed, &probe) == nil {
			return trimmed
		}
	}
	start := bytes.IndexByte(data, '{')
	if start < 0 {
		return nil
	}
	last := bytes.LastIndexByte(data, '}')
	if last <= start {
		return nil
	}
	candidate := data[start : last+1]
	var probe map[string]json.RawMessage
	if json.Unmarshal(candidate, &probe) == nil {
		return candidate
	}
	return nil
}

// detectOpenclawPluginsViaCLI runs `openclaw plugins list --json` and returns
// the enabled plugins. Callers should fall back to the directory scan on error.
func detectOpenclawPluginsViaCLI(binPath string) ([]PluginInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "plugins", "list", "--json")
	// Config warnings go to stderr and would otherwise pollute logs.
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run openclaw plugins list --json: %w", err)
	}
	return parseOpenclawPluginsJSON(out)
}

// scanOpenclawExtensionsDir is the legacy directory-based detector kept as a
// fallback for older openclaw versions where `plugins list --json` is missing
// or broken. It only sees plugins installed under ~/.openclaw/extensions/ and
// misses npm/bundled sources — so it is strictly a safety net.
func scanOpenclawExtensionsDir() []PluginInfo {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	extDir := filepath.Join(home, ".openclaw", "extensions")
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return nil
	}

	var plugins []PluginInfo
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasSuffix(entry.Name(), ".bak") || entry.Name() == "node_modules" {
			continue
		}
		pkgPath := filepath.Join(extDir, entry.Name(), "package.json")
		data, err := os.ReadFile(pkgPath)
		if err != nil {
			continue
		}
		var pkg struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &pkg) != nil || pkg.Name == "" {
			continue
		}
		plugins = append(plugins, PluginInfo{
			Name:    pkg.Name,
			Version: pkg.Version,
		})
	}
	return plugins
}

// DetectOpenclawPlugins is the authoritative plugin detector. It prefers the
// CLI (covers bundled + npm + global + extensions sources) and falls back to
// scanning ~/.openclaw/extensions/ when the CLI fails or is unsupported.
func DetectOpenclawPlugins(binPath string) []PluginInfo {
	if binPath != "" {
		plugins, err := detectOpenclawPluginsViaCLI(binPath)
		if err == nil {
			return plugins
		}
		log.Printf("[WARN] openclaw plugins list --json failed, falling back to dir scan: %v", err)
	}
	fallback := scanOpenclawExtensionsDir()
	if len(fallback) > 0 {
		log.Printf("[INFO] plugin detection fallback: found %d in ~/.openclaw/extensions/", len(fallback))
	}
	return fallback
}
