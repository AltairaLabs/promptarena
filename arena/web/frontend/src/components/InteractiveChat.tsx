import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowLeft, RotateCcw } from "lucide-react";
import { LiveConsole, Button, Card, Select, Checkbox, Input, Alert } from "@altairalabs/atlas";
import { useInteractiveChat } from "@/hooks/useInteractiveChat";
import { useVoiceCall } from "@/hooks/useVoiceCall";
import { adaptLiveMessages } from "@/lib/atlasAdapter";
import { arenaInspectorTabs } from "@/lib/arenaInspectorTabs";
import type { ArenaState } from "@/types";

interface InteractiveChatProps {
  state: ArenaState;
  registerInteractiveRun: (sessionId: string) => void;
  onBack: () => void;
}

type Phase = "setup" | "vars" | "chat";

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
  const [voiceProviders, setVoiceProviders] = useState<string[]>([]);

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

  // Voice is offered per-provider: only when the selected model supports realtime
  // audio. voiceUnavailable = the config CAN do voice, but this provider can't —
  // used to explain (rather than silently hide) why there's no call control.
  const voiceEnabled = voiceProviders.includes(selectedProvider);
  const voiceUnavailable = voiceProviders.length > 0 && !voiceEnabled;

  const voiceCall = useVoiceCall({ sessionId, enabled: voiceEnabled });

  // Load options on mount
  useEffect(() => {
    setLoadingOptions(true);
    fetchOptions()
      .then((opts) => {
        setAgents(opts.agents);
        setProviders(opts.providers);
        setHasEvals(opts.hasEvals);
        setVoiceProviders(opts.voiceProviders ?? []);
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
          setSystemPrompt(result.systemPrompt ?? null);
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

  // The turn's messages — user included — are only persisted and broadcast when
  // the turn completes, so without this the sender's own message is invisible
  // for the whole generation (measured at ~5s). Hold it locally and render it
  // immediately; the authoritative message.created replaces it on arrival.
  const [pendingUser, setPendingUser] = useState<string | null>(null);
  // The session's rendered system prompt, returned when the session opens. The
  // real system message only arrives when the first turn completes, so this
  // stands in until then.
  const [systemPrompt, setSystemPrompt] = useState<string | null>(null);

  const handleSend = useCallback(
    (text: string) => {
      if (!sessionId || !text.trim() || busy) return;
      setPendingUser(text.trim());
      void sendMessage(sessionId, text.trim());
    },
    [sessionId, busy, sendMessage],
  );

  useEffect(() => {
    if (!pendingUser || !sessionId) return;
    const msgs = state.runs[sessionId]?.messages ?? [];
    if (msgs.some((m) => m.role === "user" && m.content === pendingUser)) setPendingUser(null);
  }, [state.runs, sessionId, pendingUser]);

  const handleReset = useCallback(() => {
    setSessionId(null);
    setMissingVars([]);
    setPendingParams(null);
    setVarValues({});
    setSessionError(null);
    setPendingUser(null);
    setSystemPrompt(null);
  }, []);

  // Messages for the active session, sorted by index (upsert already sorts,
  // but defensive sort here handles any edge case), then adapted to Atlas.
  // Stored entries are LiveMessage (thin message.created fields, upgraded
  // in place to the full persisted Message once message.full arrives) — a
  // superset of Message, so adaptLiveMessages picks up metrics/meta/raw
  // fields with no extra mapping once the full event lands.
  const liveMessages = useMemo(() => {
    if (!sessionId) return [];
    // Do not bail when the run is absent. The system prompt and the just-sent
    // user turn are known to the console itself, and are the whole point of
    // showing something before the server has persisted anything — a run only
    // appears in state once its first event arrives.
    const run = state.runs[sessionId];
    const msgs = [...(run?.messages ?? [])].sort((a, b) => (a.index ?? 0) - (b.index ?? 0));
    const adapted = adaptLiveMessages(msgs);

    // Stand in for the system turn until the real one is persisted at the end
    // of the first turn. Prepended so it keeps position 0, where the server
    // will also place it.
    if (systemPrompt && !msgs.some((m) => m.role === "system")) {
      adapted.unshift({
        id: "pending-system",
        role: "system",
        sequenceNum: -1,
        timestamp: new Date().toISOString(),
        parts: [{ type: "text", text: systemPrompt }],
      } as (typeof adapted)[number]);
    }

    // Show the just-sent user turn until the real one lands. Matching on
    // content rather than index because the server assigns the index (the
    // system prompt takes 0 on the first turn) and we do not know it yet.
    if (pendingUser && !msgs.some((m) => m.role === "user" && m.content === pendingUser)) {
      adapted.push({
        id: "pending-user",
        role: "user",
        sequenceNum: msgs.length,
        timestamp: new Date().toISOString(),
        parts: [{ type: "text", text: pendingUser }],
      } as (typeof adapted)[number]);
    }

    // Append the turn currently being generated. Reasoning starts streaming
    // before message.created exists, so this is a synthetic bubble rather than
    // an update to a real message; the reducer drops `streaming` the moment the
    // authoritative message lands, and this row is replaced by it.
    // The in-flight assistant turn. Rendered from the moment the request is
    // sent rather than when the first token lands: there is a real gap before
    // anything arrives (measured ~2.5s to the first reasoning fragment), and an
    // empty pane in that window looks like nothing happened.
    //
    // `streaming: true` is what draws Atlas's cursor, so the bubble reads as
    // working even while it has no text yet.
    //
    // Dropped once the real assistant message exists, so the placeholder never
    // trails a finished turn while the request is still settling.
    const s = run?.streaming;
    const streamedReasoning = s ? s.reasoningParts.map((p) => p.text).join("") : "";
    // In flight for as long as the sent message has not been persisted. Do NOT
    // test "last message is an assistant" — after the first turn that is the
    // PREVIOUS turn's reply, which suppressed the indicator on every turn but
    // the first. pendingUser clears at the same moment the real messages land,
    // because a turn's messages are all broadcast together.
    const turnInFlight = busy && pendingUser !== null;
    if (turnInFlight) {
      // Atlas draws a streaming cursor, but only for the message whose id
      // matches MessageStream's `streamingId` prop — it ignores the message's
      // own `streaming` field, and LiveConsole does not accept or forward a
      // streamingId. So the cursor is unreachable from here and an empty
      // bubble renders as a blank assistant turn. Until Atlas forwards it
      // (AltairaLabs/atlas-components), stand in with visible text.
      const placeholder = streamedReasoning ? "Thinking…" : "…";
      adapted.push({
        id: "streaming",
        role: "assistant",
        sequenceNum: msgs.length,
        timestamp: new Date().toISOString(),
        streaming: true,
        parts: [{ type: "text", text: s?.content || placeholder }],
        ...(streamedReasoning ? { reasoning: { text: streamedReasoning } } : {}),
      } as (typeof adapted)[number]);
    }
    return adapted;
  }, [sessionId, state.runs, pendingUser, systemPrompt, busy]);

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
      <Card style={{ maxWidth: 520, margin: "0 auto" }}>
        <Alert tone="error" style={{ marginBottom: 16 }}>
          Failed to load interactive options: {optionsError}
        </Alert>
        <button onClick={onBack} style={ghostLinkStyle}>
          <ArrowLeft className="h-4 w-4" /> Back to Runs
        </button>
      </Card>
    );
  }

  // --- Setup phase ---
  if (phase === "setup") {
    return (
      <div style={{ maxWidth: 480, margin: "40px auto 0" }}>
        <Card>
          <h2 style={{ font: "600 18px var(--font-sans)", color: "var(--star-100)", margin: "0 0 6px" }}>
            Interactive Chat
          </h2>
          <p style={{ font: "400 13px/1.6 var(--font-sans)", color: "var(--star-600)", margin: "0 0 24px" }}>
            Chat live with an agent from your Arena config.
          </p>

          {sessionError && (
            <Alert tone="error" style={{ marginBottom: 16 }}>
              {sessionError}
            </Alert>
          )}

          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            {agents.length > 1 && (
              <Select
                label="Agent"
                placeholder="Select agent…"
                options={agents.map((a) => a.taskType)}
                value={selectedAgent}
                onChange={(e) => setSelectedAgent(e.target.value)}
              />
            )}

            {providers.length > 1 && (
              <Select
                label="Provider"
                placeholder="Select provider…"
                options={providers}
                value={selectedProvider}
                onChange={(e) => setSelectedProvider(e.target.value)}
              />
            )}

            {hasEvals && (
              <Checkbox
                label="Run evals per turn"
                checked={enableEvals}
                onChange={(e) => setEnableEvals(e.target.checked)}
              />
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
        </Card>
      </div>
    );
  }

  // --- Vars phase ---
  if (phase === "vars") {
    return (
      <div style={{ maxWidth: 480, margin: "40px auto 0" }}>
        <Card>
          <h2 style={{ font: "600 18px var(--font-sans)", color: "var(--star-100)", margin: "0 0 6px" }}>
            Required Variables
          </h2>
          <p style={{ font: "400 13px/1.6 var(--font-sans)", color: "var(--star-600)", margin: "0 0 24px" }}>
            The selected agent requires values for the following template variables.
          </p>

          {sessionError && (
            <Alert tone="error" style={{ marginBottom: 16 }}>
              {sessionError}
            </Alert>
          )}

          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            {missingVars.map((v) => (
              <Input
                key={v}
                label={v}
                type="text"
                value={varValues[v] ?? ""}
                onChange={(e) => setVarValues((prev) => ({ ...prev, [v]: e.target.value }))}
                placeholder={`Enter ${v}…`}
              />
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
        </Card>
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
            {voiceUnavailable && (
              <span style={{ color: "var(--text-faint)" }}>· voice needs a realtime model</span>
            )}
          </span>
        }
        headerExtra={
          <span style={{ display: "inline-flex", alignItems: "center", gap: 14 }}>
            <button onClick={handleReset} style={ghostLinkStyle}>
              <RotateCcw className="h-4 w-4" /> New chat
            </button>
            <button onClick={onBack} style={ghostLinkStyle}>
              <ArrowLeft className="h-4 w-4" /> Runs
            </button>
          </span>
        }
      />
      {error && (
        <Alert tone="error" style={{ margin: "8px 4px 0" }}>
          {error}
        </Alert>
      )}
    </div>
  );
}
