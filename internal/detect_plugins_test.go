package internal

import (
	"testing"
)

const pluginsListFixture = `{
  "registry": {"source": "persisted", "diagnostics": []},
  "plugins": [
    {
      "id": "active-memory",
      "name": "Active Memory",
      "source": "/opt/homebrew/lib/node_modules/openclaw/dist/extensions/active-memory/index.js",
      "origin": "bundled",
      "enabled": false,
      "status": "disabled"
    },
    {
      "id": "openclaw-channel-dmwork",
      "name": "openclaw-channel-dmwork",
      "version": "0.6.3-dev.dc640a2e",
      "source": "/Users/caster/.openclaw/npm/node_modules/openclaw-channel-dmwork/dist/index.js",
      "origin": "global",
      "enabled": true,
      "status": "loaded"
    },
    {
      "id": "@larksuite/openclaw-lark",
      "name": "Lark",
      "version": "2026.4.8",
      "source": "global:openclaw-lark/index.js",
      "origin": "global",
      "enabled": true,
      "status": "loaded"
    },
    {
      "id": "stub-no-version",
      "enabled": true,
      "status": "loaded"
    }
  ]
}`

func TestParseOpenclawPluginsJSON_FiltersDisabledAndMissingVersion(t *testing.T) {
	got, err := parseOpenclawPluginsJSON([]byte(pluginsListFixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 plugins, got %d: %+v", len(got), got)
	}

	byName := make(map[string]string)
	for _, p := range got {
		byName[p.Name] = p.Version
	}
	if byName["openclaw-channel-dmwork"] != "0.6.3-dev.dc640a2e" {
		t.Errorf("dmwork plugin wire name/version wrong: %+v", byName)
	}
	if byName["@larksuite/openclaw-lark"] != "2026.4.8" {
		t.Errorf("lark plugin wire name/version wrong: %+v", byName)
	}
	if _, ok := byName["active-memory"]; ok {
		t.Errorf("disabled plugin should be filtered out")
	}
	if _, ok := byName["stub-no-version"]; ok {
		t.Errorf("plugin without version should be filtered out")
	}
}

func TestParseOpenclawPluginsJSON_UsesIDAsName(t *testing.T) {
	// Regression guard for the server/frontend matching invariant:
	// PluginInfo.Name must be the openclaw plugin id (= npm package name),
	// NOT the human-readable display name. Otherwise close-out by name breaks.
	input := `{"plugins":[{"id":"openclaw-channel-dmwork","name":"Display Name","version":"1.0.0","enabled":true}]}`
	got, err := parseOpenclawPluginsJSON([]byte(input))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Name != "openclaw-channel-dmwork" {
		t.Errorf("PluginInfo.Name should carry id, got %q", got[0].Name)
	}
}

func TestParseOpenclawPluginsJSON_EmptyPlugins(t *testing.T) {
	got, err := parseOpenclawPluginsJSON([]byte(`{"plugins":[]}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}

func TestParseOpenclawPluginsJSON_NoPluginsField(t *testing.T) {
	// Object without plugins key — no error, empty slice.
	got, err := parseOpenclawPluginsJSON([]byte(`{"registry":{}}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}

func TestParseOpenclawPluginsJSON_PrefixNoise(t *testing.T) {
	// Future-proof: if stdout ever prefixes log lines before the JSON object.
	input := `[openclaw] booting extensions...
warning: something
{"plugins":[{"id":"openclaw-channel-dmwork","version":"0.6.0","enabled":true}]}`
	got, err := parseOpenclawPluginsJSON([]byte(input))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 || got[0].Name != "openclaw-channel-dmwork" {
		t.Errorf("expected dmwork plugin, got %+v", got)
	}
}

func TestParseOpenclawPluginsJSON_Malformed(t *testing.T) {
	_, err := parseOpenclawPluginsJSON([]byte(`not json at all`))
	if err == nil {
		t.Error("expected error on malformed input")
	}
}

func TestExtractJSONObject_Trivial(t *testing.T) {
	if got := extractJSONObject([]byte(`{"a":1}`)); string(got) != `{"a":1}` {
		t.Errorf("got %q", got)
	}
}

func TestExtractJSONObject_WithPrefix(t *testing.T) {
	input := `noise line
{"a":1}`
	got := extractJSONObject([]byte(input))
	if string(got) != `{"a":1}` {
		t.Errorf("got %q", got)
	}
}

func TestExtractJSONObject_NoBraces(t *testing.T) {
	if got := extractJSONObject([]byte(`no json`)); got != nil {
		t.Errorf("expected nil, got %q", got)
	}
}
