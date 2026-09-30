package mcp

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/mcp"
)

// TestNewRegistryFromConfig_CarriesEveryServerField builds a registry through
// the public entry point and checks the runtime ServerConfig it registered,
// field by field, for both an HTTP server and a stdio server.
func TestNewRegistryFromConfig_CarriesEveryServerField(t *testing.T) {
	cfg := &arenaconfig.Config{
		MCPServers: []config.MCPServerConfig{
			{
				Name:       "remote",
				URL:        "https://mcp.example.com/mcp",
				Headers:    map[string]string{"Authorization": "Bearer token-123"},
				Transport:  "streamable_http",
				TimeoutMs:  2500,
				ToolFilter: &config.MCPToolFilter{Allowlist: []string{"search"}},
			},
			{
				Name:       "local",
				Command:    "node",
				Args:       []string{"server.js"},
				Env:        map[string]string{"NODE_ENV": "test"},
				WorkingDir: "/srv/mcp",
				ToolFilter: &config.MCPToolFilter{Blocklist: []string{"delete"}},
			},
		},
	}

	registry, err := NewRegistryFromConfig(cfg)
	require.NoError(t, err)

	remote, ok := registry.GetServerConfig("remote")
	require.True(t, ok, "remote server should be registered")
	assert.Equal(t, mcp.ServerConfig{
		Name:          "remote",
		URL:           "https://mcp.example.com/mcp",
		Headers:       map[string]string{"Authorization": "Bearer token-123"},
		TransportName: mcp.TransportStreamableHTTP,
		TimeoutMs:     2500,
		ToolFilter:    &mcp.ToolFilter{Allowlist: []string{"search"}},
	}, remote)
	assert.Equal(t, mcp.TransportStreamableHTTP, remote.Transport())

	local, ok := registry.GetServerConfig("local")
	require.True(t, ok, "local server should be registered")
	assert.Equal(t, mcp.ServerConfig{
		Name:       "local",
		Command:    "node",
		Args:       []string{"server.js"},
		Env:        map[string]string{"NODE_ENV": "test"},
		WorkingDir: "/srv/mcp",
		ToolFilter: &mcp.ToolFilter{Blocklist: []string{"delete"}},
	}, local)
}

// TestNewRegistryFromConfig_URLWithoutTransportDefaultsToSSE pins the
// back-compat default: a url with no explicit transport resolves to SSE.
func TestNewRegistryFromConfig_URLWithoutTransportDefaultsToSSE(t *testing.T) {
	cfg := &arenaconfig.Config{
		MCPServers: []config.MCPServerConfig{{Name: "legacy", URL: "http://localhost:9000"}},
	}

	registry, err := NewRegistryFromConfig(cfg)
	require.NoError(t, err)

	sc, ok := registry.GetServerConfig("legacy")
	require.True(t, ok)
	assert.Equal(t, "http://localhost:9000", sc.URL)
	assert.Equal(t, mcp.TransportSSE, sc.Transport())
}

// TestNewRegistryFromConfig_SkipsSourceBackedServers checks that a
// source-backed entry is not registered as an endpoint-less server; its
// MCPSource provides the endpoint when the engine opens it.
func TestNewRegistryFromConfig_SkipsSourceBackedServers(t *testing.T) {
	cfg := &arenaconfig.Config{
		MCPServers: []config.MCPServerConfig{
			{Name: "sandbox", Source: "docker", Scope: "session"},
			{Name: "static", Command: "npx"},
		},
	}

	registry, err := NewRegistryFromConfig(cfg)
	require.NoError(t, err)

	_, ok := registry.GetServerConfig("sandbox")
	assert.False(t, ok, "source-backed server must not be registered without an endpoint")
	assert.Equal(t, []string{"static"}, registry.ListServers())
}

// unmappedMCPServerFields are the config.MCPServerConfig fields that have no
// runtime ServerConfig counterpart. They steer the arena engine's MCPSource
// provisioning instead. Adding a field here is a deliberate decision.
var unmappedMCPServerFields = map[string]bool{
	"Source":     true,
	"Scope":      true,
	"SourceArgs": true,
}

// TestServerConfigFromConfig_MapsEveryField is a coverage guard: it fails when
// PromptKit adds a field to config.MCPServerConfig or mcp.ServerConfig that
// the conversion does not carry across.
func TestServerConfigFromConfig_MapsEveryField(t *testing.T) {
	srcType := reflect.TypeOf(config.MCPServerConfig{})

	t.Run("every source field reaches the runtime config", func(t *testing.T) {
		for i := 0; i < srcType.NumField(); i++ {
			field := srcType.Field(i)
			if unmappedMCPServerFields[field.Name] {
				continue
			}
			var entry config.MCPServerConfig
			fillNonZero(t, reflect.ValueOf(&entry).Elem().Field(i))

			got := ServerConfigFromConfig(&entry)
			assert.False(t, reflect.ValueOf(got).IsZero(),
				"config.MCPServerConfig.%s is not mapped to mcp.ServerConfig; map it in "+
					"ServerConfigFromConfig or list it in unmappedMCPServerFields", field.Name)
		}
	})

	t.Run("every runtime field is populated", func(t *testing.T) {
		var entry config.MCPServerConfig
		fillNonZero(t, reflect.ValueOf(&entry).Elem())

		got := reflect.ValueOf(ServerConfigFromConfig(&entry))
		for i := 0; i < got.NumField(); i++ {
			assert.False(t, got.Field(i).IsZero(),
				"mcp.ServerConfig.%s is never set by ServerConfigFromConfig", got.Type().Field(i).Name)
		}
	})

	t.Run("unmapped fields exist", func(t *testing.T) {
		for name := range unmappedMCPServerFields {
			_, ok := srcType.FieldByName(name)
			assert.True(t, ok, "unmappedMCPServerFields lists %s, which config.MCPServerConfig no longer has", name)
		}
	})
}

// fillNonZero sets v (and everything it contains) to a non-zero value.
func fillNonZero(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Int, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Interface:
		v.Set(reflect.ValueOf("x"))
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillNonZero(t, s.Index(0))
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		fillNonZero(t, key)
		val := reflect.New(v.Type().Elem()).Elem()
		fillNonZero(t, val)
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		fillNonZero(t, p.Elem())
		v.Set(p)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			fillNonZero(t, v.Field(i))
		}
	default:
		t.Fatalf("fillNonZero: unsupported kind %s; extend the helper", v.Kind())
	}
}
