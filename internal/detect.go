package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var versionRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

type RuntimeInfo struct {
	Provider string       `json:"type"`
	Name     string       `json:"name"`
	Version  string       `json:"version"`
	Status   string       `json:"status"`
	Path     string       `json:"-"`
	Agents   []AgentEntry `json:"agents,omitempty"`
	Plugins  []PluginInfo `json:"plugins,omitempty"`
}

type AgentEntry struct {
	ID       string   `json:"id"`
	Name     string   `json:"name,omitempty"`
	Bindings int      `json:"bindings"`
	Default  bool     `json:"is_default"`
	Routes   []string `json:"routes,omitempty"`
}

var providers = map[string]string{
	"claude":   "claude",
	"codex":    "codex",
	"openclaw": "openclaw",
	"hermes":   "hermes",
}

func DetectRuntimes() []RuntimeInfo {
	var runtimes []RuntimeInfo
	for provider, binary := range providers {
		binPath, err := exec.LookPath(binary)
		if err != nil {
			continue
		}
		version := detectVersion(binPath)
		status := "online"
		if provider == "openclaw" {
			gwRunning := isOpenclawGatewayRunning(binPath)
			log.Printf("[DEBUG] openclaw gateway running: %v", gwRunning)
			if !gwRunning {
				status = "offline"
			}
		}
		rt := RuntimeInfo{
			Provider: provider,
			Name:     provider,
			Version:  version,
			Status:   status,
			Path:     binPath,
		}
		if provider == "openclaw" {
			rt.Agents = DetectOpenclawAgents(binPath)
			rt.Plugins = detectOpenclawPlugins()
		}
		runtimes = append(runtimes, rt)
	}
	return runtimes
}

func DetectRuntimesWithDeviceName(deviceName string) []RuntimeInfo {
	runtimes := DetectRuntimes()
	for i := range runtimes {
		runtimes[i].Name = fmt.Sprintf("%s (%s)", capitalize(runtimes[i].Provider), deviceName)
	}
	return runtimes
}

type openclawAgentJSON struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Bindings  int      `json:"bindings"`
	IsDefault bool     `json:"isDefault"`
	Routes    []string `json:"routes"`
}

func DetectOpenclawAgents(binPath string) []AgentEntry {
	if binPath == "" {
		p, err := exec.LookPath("openclaw")
		if err != nil {
			return nil
		}
		binPath = p
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "agents", "list", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Strip non-JSON prefix lines (e.g. "[dmwork] registering ...")
	// Find the line that is exactly "[" (the JSON array start)
	out = extractJSONArray(out)
	if out == nil {
		return nil
	}

	var raw []openclawAgentJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}

	agents := make([]AgentEntry, 0, len(raw))
	for _, a := range raw {
		name := a.Name
		if name == "" {
			name = a.ID
		}
		agents = append(agents, AgentEntry{
			ID:       a.ID,
			Name:     name,
			Bindings: a.Bindings,
			Default:  a.IsDefault,
			Routes:   a.Routes,
		})
	}
	return agents
}

func detectVersion(binPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	raw := strings.TrimSpace(string(out))
	if m := versionRe.FindString(raw); m != "" {
		return m
	}
	return raw
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// isOpenclawGatewayRunning parses `openclaw gateway status` output to determine
// if the gateway is actually running. It checks the "Probe target" URL and
// probes that port, respecting whatever IP/port the user has configured.
func isOpenclawGatewayRunning(binPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "gateway", "status")
	out, _ := cmd.CombinedOutput()
	if len(out) == 0 {
		return false
	}

	output := string(out)

	// Parse "Probe target: ws://host:port" to get the actual address
	for _, line := range strings.Split(output, "\n") {
		// Strip ANSI escape codes
		clean := stripAnsi(line)
		clean = strings.TrimSpace(clean)
		if strings.HasPrefix(clean, "Probe target:") {
			target := strings.TrimSpace(strings.TrimPrefix(clean, "Probe target:"))
			target = strings.TrimPrefix(target, "ws://")
			target = strings.TrimPrefix(target, "wss://")
			if target != "" {
				conn, dialErr := net.DialTimeout("tcp", target, 2*time.Second)
				if dialErr != nil {
					log.Printf("[DEBUG] openclaw probe %s failed: %v", target, dialErr)
					return false
				}
				conn.Close()
				return true
			}
		}
	}

	log.Printf("[DEBUG] openclaw gateway status: no 'Probe target' found in output")
	return false
}

func stripAnsi(s string) string {
	const ansiEscape = '\033'
	var result []byte
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == byte(ansiEscape) {
			inEscape = true
			continue
		}
		if inEscape {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inEscape = false
			}
			continue
		}
		result = append(result, s[i])
	}
	return string(result)
}

type PluginInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func detectOpenclawPlugins() []PluginInfo {
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

// extractJSONArray finds a JSON array in output that may have non-JSON prefix lines.
// Looks for a line that is exactly "[" (trimmed) to start the JSON block.
func extractJSONArray(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	start := -1
	end := -1
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if start == -1 && bytes.Equal(trimmed, []byte("[")) {
			start = i
		}
		if start != -1 && bytes.Equal(trimmed, []byte("]")) {
			end = i
		}
	}
	if start == -1 || end == -1 {
		return nil
	}
	return bytes.Join(lines[start:end+1], []byte("\n"))
}
