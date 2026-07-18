import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowLeft } from "lucide-react";
import { LiveConsole, Button } from "@altairalabs/atlas";
import { useInteractiveChat } from "@/hooks/useInteractiveChat";
import { useVoiceCall } from "@/hooks/useVoiceCall";
import { adaptLiveMessages } from "@/lib/atlasAdapter";
import { arenaInspectorTabs } from "@/lib/arenaInspectorTabs";
import type { ArenaState, MessageCreatedData, Message } from "@/types";

interface InteractiveChatProps {
  state: ArenaState;
  registerInteractiveRun: (sessionId: string) => void;
  onBack: () => void;
}

// liveMessageToMessage maps in-flight SSE MessageCreatedData to the Message
// shape ConversationThread renders. Mirrors the same helper in RunDetail.
function liveMessageToMessage(m: MessageCreatedData): Message {
  return {
    role: m.role,
    content: m.content,
    tool_calls: m.toolCalls,
    tool_result: m.toolResult ?? undefined,
  };
}

type Phase = "setup" | "vars" | "chat";

// Atlas token styles shared by the setup/vars phase cards — mirrors the
// ink-surface card chrome used by CommandStrip/InstrumentBand/TrialMatrix.
const cardStyle: React.CSSProperties = {
  border: "1px solid var(--hairline)",
  borderRadius: "var(--radius-2xl)",
  background: "var(--grad-surface)",
  padding: 32,
};

const labelStyle: React.CSSProperties = {
  display: "block",
  font: "500 12px var(--font-mono)",
  textTransform: "uppercase",
  letterSpacing: "0.08em",
  color: "var(--star-800)",
  marginBottom: 6,
};

const selectStyle: React.CSSProperties = {
  width: "100%",
  borderRadius: "var(--radius-md)",
  border: "1px solid var(--hairline-strong)",
  background: "var(--ink-raised)",
  color: "var(--star-300)",
  padding: "9px 12px",
  font: "400 13px var(--font-sans)",
};

const errorBannerStyle: React.CSSProperties = {
  marginBottom: 16,
  border: "1px solid rgba(239,68,68,0.3)",
  borderRadius: "var(--radius-md)",
  background: "color-mix(in srgb, var(--signal-red) 12%, transparent)",
  padding: "10px 14px",
  font: "400 13px/1.5 var(--font-sans)",
  color: "var(--signal-red-300)",
};

const ghostLinkStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 6,
  font: "500 12px var(--font-mono)",
  color: "var(--starlight-300)",
  background: "transparent",
  border: "none",
  cursor: "pointer",
  padding: 0,
};

export function InteractiveChat({ state, registerInteractiveRun, onBack }: InteractiveChatProps) {
  const { fetchOptions, createSession, sendMessage, busy, error } = useInteractiveChat();

  // Setup phase state
  const [loadingOptions, setLoadingOptions] = useState(true);
  const [optionsError, setOptionsError] = useState<string | null>(null);
  const [agents, setAgents] = useState<Array<{ taskType: string; description: string }>>([]);
  const [providers, setProviders] = useState<string[]>([]);
  const [hasEvals, setHasEvals] = useState(false);
  const [voiceEnabled, setVoiceEnabled] = useState(false);

  const [selectedAgent, setSelectedAgent] = useState<string>("");
  const [selectedProvider, setSelectedProvider] = useState<string>("");
  const [enableEvals, setEnableEvals] = useState(false);
  const [sessionCreating, setSessionCreating] = useState(false);
  const [sessionError, setSessionError] = useState<string | null>(null);

  // Vars phase state
  const [missingVars, setMissingVars] = useState<string[]>([]);
  const [varValues, setVarValues] = useState<Record<string, string>>({});
  const [pendingParams, setPendingParams] = useState<{
    agent: string;
    provider: string;
    evals: boolean;
  } | null>(null);

  // Chat phase state
  const [sessionId, setSessionId] = useState<string | null>(null);

  const phase: Phase = sessionId ? "chat" : missingVars.length > 0 ? "vars" : "setup";

  const voiceCall = useVoiceCall({ sessionId, enabled: voiceEnabled });

  // Load options on mount
  useEffect(() => {
    setLoadingOptions(true);
    fetchOptions()
      .then((opts) => {
        setAgents(opts.agents);
        setProviders(opts.providers);
        setHasEvals(opts.hasEvals);
        setVoiceEnabled(opts.voice);
        if (opts.agents.length === 1) setSelectedAgent(opts.agents[0].taskType);
        if (opts.providers.length === 1) setSelectedProvider(opts.providers[0]);
      })
      .catch((e: Error) => setOptionsError(e.message))
      .finally(() => setLoadingOptions(false));
  }, [fetchOptions]);

  const doCreateSession = useCallback(
    async (agent: string, provider: string, variables: Record<string, string>, evals: boolean) => {
      setSessionCreating(true);
      setSessionError(null);
      try {
        const result = await createSession({ agent, provider, variables, evals });
        if (result.error) {
          setSessionError(result.error);
        } else if (result.missingVars && result.missingVars.length > 0) {
          setMissingVars(result.missingVars);
          setPendingParams({ agent, provider, evals });
          const init: Record<string, string> = {};
          for (const v of result.missingVars) init[v] = "";
          setVarValues(init);
        } else if (result.sessionId) {
          registerInteractiveRun(result.sessionId);
          setSessionId(result.sessionId);
        }
      } finally {
        setSessionCreating(false);
      }
    },
    [createSession, registerInteractiveRun],
  );

  const handleSetupSubmit = useCallback(async () => {
    if (!selectedAgent || !selectedProvider) return;
    await doCreateSession(selectedAgent, selectedProvider, {}, enableEvals);
  }, [selectedAgent, selectedProvider, enableEvals, doCreateSession]);

  const handleVarsSubmit = useCallback(async () => {
    if (!pendingParams) return;
    await doCreateSession(pendingParams.agent, pendingParams.provider, varValues, pendingParams.evals);
  }, [pendingParams, varValues, doCreateSession]);

  const handleSend = useCallback(
    (text: string) => {
      if (!sessionId || !text.trim() || busy) return;
      void sendMessage(sessionId, text.trim());
    },
    [sessionId, busy, sendMessage],
  );

  const handleReset = useCallback(() => {
    setSessionId(null);
    setMissingVars([]);
    setPendingParams(null);
    setVarValues({});
    setSessionError(null);
  }, []);

  // Messages for the active session, sorted by index (upsert already sorts,
  // but defensive sort here handles any edge case), then adapted to Atlas.
  const liveMessages = useMemo(() => {
    if (!sessionId) return [];
    const run = state.runs[sessionId];
    if (!run?.messages?.length) return [];
    const msgs: Message[] = [...run.messages]
      .sort((a, b) => (a.index ?? 0) - (b.index ?? 0))
      .map(liveMessageToMessage);
    return adaptLiveMessages(msgs);
  }, [sessionId, state.runs]);

  if (loadingOptions) {
    return (
      <div style={{ display: "flex", alignItems: "center", justifyContent: "center", padding: "96px 0" }}>
        <div style={{ font: "400 13px var(--font-mono)", color: "var(--star-700)" }} className="animate-pulse">
          Loading options…
        </div>
      </div>
    );
  }

  if (optionsError) {
    return (
      <div style={{ ...cardStyle, maxWidth: 520, margin: "0 auto" }}>
        <p style={{ font: "400 13px/1.6 var(--font-sans)", color: "var(--signal-red-300)", marginBottom: 16 }}>
          Failed to load interactive options: {optionsError}
        </p>
        <button onClick={onBack} style={ghostLinkStyle}>
          <ArrowLeft className="h-4 w-4" /> Back to Runs
        </button>
      </div>
    );
  }

  // --- Setup phase ---
  if (phase === "setup") {
    return (
      <div style={{ maxWidth: 480, margin: "40px auto 0" }}>
        <div style={cardStyle}>
          <h2 style={{ font: "600 18px var(--font-sans)", color: "var(--star-100)", margin: "0 0 6px" }}>
            Interactive Chat
          </h2>
          <p style={{ font: "400 13px/1.6 var(--font-sans)", color: "var(--star-600)", margin: "0 0 24px" }}>
            Chat live with an agent from your Arena config.
          </p>

          {sessionError && <div style={errorBannerStyle}>{sessionError}</div>}

          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            {agents.length > 1 && (
              <div>
                <label style={labelStyle}>Agent</label>
                <select
                  style={selectStyle}
                  value={selectedAgent}
                  onChange={(e) => setSelectedAgent(e.target.value)}
                >
                  <option value="">Select agent…</option>
                  {agents.map((a) => (
                    <option key={a.taskType} value={a.taskType}>
                      {a.taskType}
                    </option>
                  ))}
                </select>
              </div>
            )}

            {providers.length > 1 && (
              <div>
                <label style={labelStyle}>Provider</label>
                <select
                  style={selectStyle}
                  value={selectedProvider}
                  onChange={(e) => setSelectedProvider(e.target.value)}
                >
                  <option value="">Select provider…</option>
                  {providers.map((p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ))}
                </select>
              </div>
            )}

            {hasEvals && (
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <input
                  id="enable-evals"
                  type="checkbox"
                  checked={enableEvals}
                  onChange={(e) => setEnableEvals(e.target.checked)}
                  style={{ accentColor: "var(--starlight-500)" }}
                />
                <label htmlFor="enable-evals" style={{ font: "400 13px var(--font-sans)", color: "var(--star-400)" }}>
                  Run evals per turn
                </label>
              </div>
            )}

            <Button
              variant="secondary"
              style={{ width: "100%" }}
              disabled={!selectedAgent || !selectedProvider || sessionCreating}
              onClick={() => void handleSetupSubmit()}
            >
              {sessionCreating ? "Starting…" : "Start Chat"}
            </Button>
          </div>
        </div>
      </div>
    );
  }

  // --- Vars phase ---
  if (phase === "vars") {
    return (
      <div style={{ maxWidth: 480, margin: "40px auto 0" }}>
        <div style={cardStyle}>
          <h2 style={{ font: "600 18px var(--font-sans)", color: "var(--star-100)", margin: "0 0 6px" }}>
            Required Variables
          </h2>
          <p style={{ font: "400 13px/1.6 var(--font-sans)", color: "var(--star-600)", margin: "0 0 24px" }}>
            The selected agent requires values for the following template variables.
          </p>

          {sessionError && <div style={errorBannerStyle}>{sessionError}</div>}

          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            {missingVars.map((v) => (
              <div key={v}>
                <label style={labelStyle}>{v}</label>
                <input
                  type="text"
                  style={selectStyle}
                  value={varValues[v] ?? ""}
                  onChange={(e) => setVarValues((prev) => ({ ...prev, [v]: e.target.value }))}
                  placeholder={`Enter ${v}…`}
                />
              </div>
            ))}

            <div style={{ display: "flex", gap: 12 }}>
              <Button variant="secondary" style={{ flex: 1 }} onClick={handleReset}>
                Back
              </Button>
              <Button
                variant="secondary"
                style={{ flex: 1 }}
                disabled={missingVars.some((v) => !varValues[v]?.trim()) || sessionCreating}
                onClick={() => void handleVarsSubmit()}
              >
                {sessionCreating ? "Starting…" : "Start Chat"}
              </Button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // --- Chat phase ---
  return (
    <div style={{ display: "flex", flexDirection: "column", height: "calc(100vh - 220px)", minHeight: 500 }}>
      <LiveConsole
        messages={liveMessages}
        inspectorTabs={arenaInspectorTabs}
        onSend={handleSend}
        call={voiceEnabled ? voiceCall : undefined}
        connectionStatus={state.connected ? "connected" : "connecting"}
        composerDisabled={busy}
        composerPlaceholder="Type a message… (Enter to send, Shift+Enter for newline)"
        title={
          <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
            <span style={{ fontWeight: 600 }}>{selectedAgent}</span>
            <span style={{ color: "var(--text-faint)" }}>·</span>
            <span style={{ color: "var(--text-muted)" }}>{selectedProvider}</span>
            {enableEvals && <span style={{ color: "var(--pulsar-300)" }}>· evals on</span>}
          </span>
        }
        headerExtra={
          <span style={{ display: "inline-flex", alignItems: "center", gap: 14 }}>
            <button onClick={handleReset} style={ghostLinkStyle}>
              <ArrowLeft className="h-4 w-4" /> Reset
            </button>
            <button onClick={onBack} style={ghostLinkStyle}>
              ← Runs
            </button>
          </span>
        }
      />
      {error && <div style={{ ...errorBannerStyle, margin: "8px 4px 0" }}>{error}</div>}
    </div>
  );
}
