package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/a2a"
	a2amock "github.com/AltairaLabs/PromptKit/runtime/v2/a2a/mock"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// a2aRequestHeaders applies opt to a real a2a.Client, makes it discover an
// httptest server, and returns the headers that server received. It is how
// the resolvers below are checked: a ClientOption is an opaque func, so the
// only honest view of what it does is the request it produces.
func a2aRequestHeaders(t *testing.T, opt a2a.ClientOption) http.Header {
	t.Helper()
	got := make(chan http.Header, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case got <- r.Header.Clone():
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"probe"}`))
	}))
	defer srv.Close()

	var opts []a2a.ClientOption
	if opt != nil {
		opts = append(opts, opt)
	}
	_, _ = a2a.NewClient(srv.URL, opts...).Discover(context.Background())
	select {
	case h := <-got:
		return h
	default:
		t.Fatal("the a2a client sent no request")
		return nil
	}
}

func TestResolveA2AAuth(t *testing.T) {
	t.Setenv("TEST_A2A_TOKEN_RESOLVE", "env-token")
	tests := []struct {
		name string
		auth *arenaconfig.A2AAuthConfig
		// wantHeader is the Authorization header the agent receives; "" means
		// the resolver must return no option at all.
		wantHeader string
	}{
		{name: "nil auth sends none", auth: nil},
		{name: "empty token sends none", auth: &arenaconfig.A2AAuthConfig{Scheme: "Bearer"}},
		{name: "unset env sends none",
			auth: &arenaconfig.A2AAuthConfig{Scheme: "Bearer", TokenEnv: "UNSET_A2A_TOKEN_XXXXX"}},
		{name: "token from config",
			auth:       &arenaconfig.A2AAuthConfig{Scheme: "Bearer", Token: "my-token"},
			wantHeader: "Bearer my-token"},
		{name: "token from env keeps its scheme",
			auth:       &arenaconfig.A2AAuthConfig{Scheme: "ApiKey", TokenEnv: "TEST_A2A_TOKEN_RESOLVE"},
			wantHeader: "ApiKey env-token"},
		{name: "token_env wins over an inline token",
			auth: &arenaconfig.A2AAuthConfig{
				Scheme: "Bearer", Token: "inline", TokenEnv: "TEST_A2A_TOKEN_RESOLVE"},
			wantHeader: "Bearer env-token"},
		{name: "empty scheme defaults to Bearer",
			auth:       &arenaconfig.A2AAuthConfig{Token: "tok"},
			wantHeader: "Bearer tok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := resolveA2AAuth(tt.auth)
			if tt.wantHeader == "" {
				assert.Nil(t, opt)
				return
			}
			assert.Equal(t, tt.wantHeader, a2aRequestHeaders(t, opt).Get("Authorization"))
		})
	}
}

func TestResolveA2AHeadersEngine(t *testing.T) {
	t.Setenv("TEST_A2A_HDR_VAL", "secret")
	tests := []struct {
		name    string
		static  map[string]string
		fromEnv []string
		// want is the set of headers the agent receives; nil means the
		// resolver must return no option at all.
		want map[string]string
	}{
		{name: "no headers sends none"},
		{name: "unset env header is skipped, leaving none",
			fromEnv: []string{"X-Key=UNSET_HDR_XXXX"}},
		{name: "static headers only",
			static: map[string]string{"X-Tenant": "acme"},
			want:   map[string]string{"X-Tenant": "acme"}},
		{name: "env headers resolved",
			fromEnv: []string{"X-Key=TEST_A2A_HDR_VAL"},
			want:    map[string]string{"X-Key": "secret"}},
		{name: "mixed static and env",
			static:  map[string]string{"X-Static": "val"},
			fromEnv: []string{"X-Dynamic=TEST_A2A_HDR_VAL", "X-Gone=UNSET_HDR_XXXX"},
			want:    map[string]string{"X-Static": "val", "X-Dynamic": "secret"}},
		{name: "an env header overrides a static one of the same name",
			static:  map[string]string{"X-Key": "static"},
			fromEnv: []string{"X-Key=TEST_A2A_HDR_VAL"},
			want:    map[string]string{"X-Key": "secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt, err := resolveA2AHeaders(tt.static, tt.fromEnv)
			require.NoError(t, err)
			if tt.want == nil {
				assert.Nil(t, opt)
				return
			}
			got := a2aRequestHeaders(t, opt)
			for k, v := range tt.want {
				assert.Equal(t, v, got.Get(k), "header %s", k)
			}
			assert.Empty(t, got.Get("X-Gone"), "an unset env var must not send an empty header")
		})
	}

	t.Run("invalid format returns error", func(t *testing.T) {
		_, err := resolveA2AHeaders(nil, []string{"no-equals"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid headers_from_env format")
	})
}

func TestBuildA2AMockConfig(t *testing.T) {
	t.Run("basic agent config", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			Name: "test-agent",
			Card: arenaconfig.A2ACardConfig{
				Name:        "Test Agent",
				Description: "A test agent",
				Skills: []arenaconfig.A2ASkillConfig{
					{ID: "echo", Name: "Echo", Description: "Echo messages", Tags: []string{"utility"}},
				},
			},
		}
		mockCfg := buildA2AMockConfig(agentCfg)

		assert.Equal(t, "test-agent", mockCfg.Name)
		assert.Equal(t, "Test Agent", mockCfg.Card.Name)
		assert.Equal(t, "A test agent", mockCfg.Card.Description)
		require.Len(t, mockCfg.Card.Skills, 1)
		assert.Equal(t, "echo", mockCfg.Card.Skills[0].ID)
		assert.Equal(t, []string{"utility"}, mockCfg.Card.Skills[0].Tags)
	})

	t.Run("with responses and match rules", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			Name: "echo-agent",
			Card: arenaconfig.A2ACardConfig{Name: "Echo"},
			Responses: []arenaconfig.A2AResponseRule{
				{
					Skill: "echo",
					Match: &arenaconfig.A2AMatchConfig{Contains: "hello", Regex: "^hello"},
					Response: &arenaconfig.A2AResponseConfig{
						Parts: []arenaconfig.A2APartConfig{{Text: "Hello back!"}},
					},
				},
				{
					Skill: "echo",
					Error: "not found",
				},
			},
		}
		mockCfg := buildA2AMockConfig(agentCfg)

		require.Len(t, mockCfg.Responses, 2)
		assert.Equal(t, "echo", mockCfg.Responses[0].Skill)
		require.NotNil(t, mockCfg.Responses[0].Match)
		assert.Equal(t, "hello", mockCfg.Responses[0].Match.Contains)
		assert.Equal(t, "^hello", mockCfg.Responses[0].Match.Regex)
		require.NotNil(t, mockCfg.Responses[0].Response)
		require.Len(t, mockCfg.Responses[0].Response.Parts, 1)
		assert.Equal(t, "Hello back!", mockCfg.Responses[0].Response.Parts[0].Text)

		assert.Equal(t, "not found", mockCfg.Responses[1].Error)
		assert.Nil(t, mockCfg.Responses[1].Match)
		assert.Nil(t, mockCfg.Responses[1].Response)
	})
}

func TestPopulateConfigCard(t *testing.T) {
	t.Run("populates empty card from discovered card", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{}
		card := &a2a.AgentCard{
			Name:        "Remote Agent",
			Description: "Discovered description",
			Skills: []a2a.AgentSkill{
				{ID: "s1", Name: "Skill One", Description: "First skill", Tags: []string{"tag1"}},
				{ID: "s2", Name: "Skill Two", Description: "Second skill"},
			},
		}
		populateConfigCard(agentCfg, card)

		assert.Equal(t, "Remote Agent", agentCfg.Card.Name)
		assert.Equal(t, "Discovered description", agentCfg.Card.Description)
		require.Len(t, agentCfg.Card.Skills, 2)
		assert.Equal(t, "s1", agentCfg.Card.Skills[0].ID)
		assert.Equal(t, "Skill One", agentCfg.Card.Skills[0].Name)
		assert.Equal(t, []string{"tag1"}, agentCfg.Card.Skills[0].Tags)
		assert.Equal(t, "s2", agentCfg.Card.Skills[1].ID)
	})

	t.Run("does not overwrite existing card fields", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			Card: arenaconfig.A2ACardConfig{
				Name:        "Custom Name",
				Description: "Custom Description",
				Skills:      []arenaconfig.A2ASkillConfig{{ID: "custom"}},
			},
		}
		card := &a2a.AgentCard{
			Name:        "Discovered Name",
			Description: "Discovered Description",
			Skills:      []a2a.AgentSkill{{ID: "discovered"}},
		}
		populateConfigCard(agentCfg, card)

		assert.Equal(t, "Custom Name", agentCfg.Card.Name)
		assert.Equal(t, "Custom Description", agentCfg.Card.Description)
		require.Len(t, agentCfg.Card.Skills, 1)
		assert.Equal(t, "custom", agentCfg.Card.Skills[0].ID)
	})

	t.Run("empty discovered skills does not overwrite", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{}
		card := &a2a.AgentCard{Name: "Agent", Skills: nil}
		populateConfigCard(agentCfg, card)

		assert.Equal(t, "Agent", agentCfg.Card.Name)
		assert.Empty(t, agentCfg.Card.Skills)
	})
}

func TestBuildRemoteA2AServer(t *testing.T) {
	t.Run("basic remote agent", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			URL:  "https://agent.example.com/a2a",
			Name: "remote",
		}
		rs, err := buildRemoteA2AServer(agentCfg)

		require.NoError(t, err)
		assert.Equal(t, "https://agent.example.com/a2a", rs.url)
		assert.Empty(t, rs.clientOpts)
		assert.Nil(t, rs.auth)
		assert.Nil(t, rs.skillFilter)
	})

	t.Run("with auth token", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			URL:  "https://agent.example.com/a2a",
			Auth: &arenaconfig.A2AAuthConfig{Scheme: "Bearer", Token: "tok123"},
		}
		rs, err := buildRemoteA2AServer(agentCfg)

		require.NoError(t, err)
		assert.Len(t, rs.clientOpts, 1)
		require.NotNil(t, rs.auth)
		assert.Equal(t, "Bearer", rs.auth.Scheme)
		assert.Equal(t, "tok123", rs.auth.Token)
	})

	t.Run("with auth from env", func(t *testing.T) {
		t.Setenv("TEST_REMOTE_TOKEN", "env-tok")
		agentCfg := &arenaconfig.A2AAgentConfig{
			URL:  "https://agent.example.com/a2a",
			Auth: &arenaconfig.A2AAuthConfig{Scheme: "Bearer", TokenEnv: "TEST_REMOTE_TOKEN"},
		}
		rs, err := buildRemoteA2AServer(agentCfg)

		require.NoError(t, err)
		assert.Len(t, rs.clientOpts, 1)
		require.NotNil(t, rs.auth)
		assert.Equal(t, "env-tok", rs.auth.Token)
	})

	t.Run("with headers", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			URL:     "https://agent.example.com/a2a",
			Headers: map[string]string{"X-Tenant": "acme"},
		}
		rs, err := buildRemoteA2AServer(agentCfg)

		require.NoError(t, err)
		assert.Len(t, rs.clientOpts, 1)
		assert.Equal(t, map[string]string{"X-Tenant": "acme"}, rs.headers)
	})

	t.Run("with skill filter", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			URL: "https://agent.example.com/a2a",
			SkillFilter: &arenaconfig.A2ASkillFilter{
				Allowlist: []string{"echo"},
				Blocklist: []string{"debug"},
			},
		}
		rs, err := buildRemoteA2AServer(agentCfg)

		require.NoError(t, err)
		require.NotNil(t, rs.skillFilter)
		assert.Equal(t, []string{"echo"}, rs.skillFilter.Allowlist)
		assert.Equal(t, []string{"debug"}, rs.skillFilter.Blocklist)
	})

	t.Run("with timeout", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			URL:       "https://agent.example.com/a2a",
			TimeoutMs: 5000,
		}
		rs, err := buildRemoteA2AServer(agentCfg)

		require.NoError(t, err)
		assert.Equal(t, 5000, rs.timeoutMs)
	})

	t.Run("invalid headers_from_env returns error", func(t *testing.T) {
		agentCfg := &arenaconfig.A2AAgentConfig{
			URL:            "https://agent.example.com/a2a",
			HeadersFromEnv: []string{"bad-format"},
		}
		_, err := buildRemoteA2AServer(agentCfg)
		assert.Error(t, err)
	})
}

func TestStartA2AServers_LocalMock(t *testing.T) {
	agentCfg := []arenaconfig.A2AAgentConfig{
		{
			Name: "test-echo",
			Card: arenaconfig.A2ACardConfig{
				Name:        "Echo",
				Description: "Echoes messages",
				Skills: []arenaconfig.A2ASkillConfig{
					{ID: "echo", Name: "echo", Description: "Echo a message"},
				},
			},
		},
	}
	servers, cleanup, err := startA2AServers(agentCfg)
	require.NoError(t, err)
	defer cleanup()

	require.Len(t, servers, 1)
	assert.NotEmpty(t, servers[0].url)
	assert.NotNil(t, servers[0].server)
	assert.False(t, servers[0].isRemote)
}

func TestDiscoverAndRegisterA2ATools_LocalMock(t *testing.T) {
	// Start a local mock A2A server.
	card := a2a.AgentCard{
		Name:               "Test Echo Agent",
		Description:        "Echoes messages",
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills: []a2a.AgentSkill{
			{ID: "echo", Name: "echo", Description: "Echo a message"},
		},
	}
	server := a2amock.NewA2AServer(&card)
	url, err := server.Start()
	require.NoError(t, err)
	defer server.Close()

	agents := []arenaconfig.A2AAgentConfig{{Name: "echo"}}
	servers := []a2aRunningServer{{url: url, server: server}}

	registry := tools.NewRegistry()
	totalTools, err := discoverAndRegisterA2ATools(servers, registry, agents)

	require.NoError(t, err)
	assert.Greater(t, totalTools, 0)
}
