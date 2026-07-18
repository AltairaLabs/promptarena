// src/types.ts

// === SSE Events ===

export interface SSEEvent {
  type: string;
  timestamp: string;
  executionId?: string;
  conversationId?: string;
  data?: unknown;
}

export interface ArenaRunStartedData {
  scenario: string;
  provider: string;
  region: string;
}

export interface ArenaRunCompletedData {
  duration: number;
  cost: number;
}

export interface ArenaRunFailedData {
  error: string;
}

export interface ArenaTurnData {
  turn_index: number;
  role: string;
  scenario: string;
  error?: string;
}

export interface ProviderCallData {
  provider: string;
  model: string;
  duration?: number;
  cost?: number;
  error?: string;
}

export interface MessageCreatedData {
  role: string;
  content: string;
  index: number;
  toolCalls?: MessageToolCall[];
  toolResult?: MessageToolResult | null;
}

// MessageFullData is the payload of the `message.full` SSE event: the same
// full, persisted Message the historical/REST path renders (role/content/
// parts/tool_calls/tool_result/timestamp/latency_ms/cost_info/finish_reason/
// meta/validations), plus the message's index in the conversation. It
// supersedes the thin MessageCreatedData for the same index once it arrives.
export interface MessageFullData {
  index: number;
  message: Message;
}

export interface MessageUpdatedData {
  index: number;
  latencyMs: number;
  inputTokens: number;
  outputTokens: number;
  totalCost: number;
}

export interface ToolCallEventData {
  toolName: string;
  callId: string;
  status: string;
}

export interface ValidationEventData {
  validatorName: string;
  validatorType: string;
  error: string;
  monitorOnly: boolean;
  score?: number;
}

// === REST API ===

export interface RunRequest {
  providers?: string[];
  scenarios?: string[];
  regions?: string[];
}

export interface RunStartedResponse {
  combinations: number;
  status: "started";
}

export interface ProviderInfo {
  id: string;
  type: string;
  model?: string;
}

export interface ScenarioInfo {
  id: string;
  description?: string;
}

export interface RunOptionsResponse {
  providers: ProviderInfo[];
  scenarios: ScenarioInfo[];
}

// === Workflow Graph ===
// Mirrors the Go backend's GET /api/workflow response exactly (no x/y —
// layout is a frontend concern, see src/lib/workflowFlow.ts's dagre pass).

export interface WorkflowGraphNode {
  id: string;
  label: string;
  kind: "entry" | "output" | "agent" | "prompt" | "tool" | "branch";
  entry: boolean;
  terminal: boolean;
  // parent is the owning state's node id for a prompt-composition "step"
  // node; unset for top-level state nodes. Mirrors the backend field.
  parent?: string;
  // dim is a frontend-only overlay field set by arenaView's
  // overlayWorkflowRun — never present in the raw backend payload.
  dim?: boolean;
}

export interface WorkflowGraphEdge {
  from: string;
  to: string;
  label?: string;
  dashed?: boolean;
  // gold/dim are frontend-only overlay fields set by arenaView's
  // overlayWorkflowRun — never present in the raw backend payload.
  gold?: boolean;
  dim?: boolean;
}

export interface WorkflowGraph {
  nodes: WorkflowGraphNode[];
  edges: WorkflowGraphEdge[];
}

// === Run Results ===

export interface RunResult {
  RunID: string;
  PromptPack: string;
  Region: string;
  ScenarioID: string;
  ProviderID: string;
  Labels?: Record<string, string>;
  Params: Record<string, unknown>;
  Messages: Message[];
  Commit: Record<string, unknown>;
  Cost: CostInfo;
  ToolStats?: ToolStats;
  Violations: ValidationError[];
  StartTime: string;
  EndTime: string;
  Duration: number;
  Error: string;
  Skipped?: boolean;
  SkipReason?: string;
  SelfPlay: boolean;
  PersonaID: string;
  AssistantRole?: SelfPlayRoleInfo;
  UserRole?: SelfPlayRoleInfo;
  MediaOutputs: MediaOutput[];
  RecordingPath?: string;
  ConversationAssertions?: AssertionsSummary;
  eval_results?: EvalResult[];
  A2AAgents: A2AAgentInfo[];
  TrialResults?: TrialResults;
}

export interface EvalResult {
  eval_id: string;
  type: string;
  score?: number;
  value?: unknown;
  metric_value?: number;
  explanation?: string;
  duration_ms: number;
  error?: string;
  message?: string;
  details?: Record<string, unknown>;
  violations?: EvalViolation[];
  skipped?: boolean;
  skip_reason?: string;
  session_id?: string;
  turn_index?: number;
}

export interface EvalViolation {
  turn_index?: number;
  description?: string;
  evidence?: Record<string, unknown>;
}

export interface Message {
  role: string;
  content: string;
  parts?: ContentPart[];
  tool_calls?: MessageToolCall[];
  tool_result?: MessageToolResult;
  timestamp?: string;
  latency_ms?: number;
  cost_info?: CostInfo;
  finish_reason?: string;
  meta?: Record<string, unknown>;
  validations?: ValidationResult[];
}

export interface ContentPart {
  type: "text" | "image" | "audio" | "video" | "document" | "thinking";
  text?: string;
  media?: MediaContent;
}

export interface MediaContent {
  mime_type: string;
  file_path?: string;
  url?: string;
  storage_reference?: string;
  format?: string;
  size_kb?: number;
  size_bytes?: number;
  duration?: number;
  width?: number;
  height?: number;
}

export interface MessageToolCall {
  id: string;
  name: string;
  args: Record<string, unknown>;
}

export interface MessageToolResult {
  id: string;
  name: string;
  error?: string;
  error_type?: string;
  parts: ContentPart[];
  latency_ms: number;
}

export interface CostInfo {
  input_tokens: number;
  output_tokens: number;
  cached_tokens?: number;
  input_cost_usd: number;
  output_cost_usd: number;
  cached_cost_usd?: number;
  total_cost_usd: number;
}

export interface ToolStats {
  total_calls: number;
  by_tool: Record<string, number>;
}

export interface ValidationError {
  type: string;
  tool: string;
  detail: string;
}

export interface ValidationResult {
  validator_type: string;
  passed: boolean;
  details?: Record<string, unknown>;
  timestamp?: string;
}

export interface AssertionsSummary {
  failed: number;
  passed: boolean;
  results: ConversationValidationResult[];
  total: number;
}

export interface ConversationValidationResult {
  name?: string;
  type: string;
  passed: boolean;
  score?: number;
  message?: string;
  details?: Record<string, unknown>;
  violations?: ConversationViolation[];
}

export interface ConversationViolation {
  turn_index: number;
  description: string;
  evidence?: Record<string, unknown>;
  timestamp?: string;
}

export interface TrialResults {
  trial_count: number;
  pass_rate: number;
  flakiness_score: number;
  per_assertion_stats?: Record<string, AssertionTrialStats>;
}

export interface AssertionTrialStats {
  pass_rate: number;
  pass_count: number;
  fail_count: number;
  flakiness_score: number;
}

export interface MediaOutput {
  Type: string;
  MIMEType: string;
  SizeBytes: number;
  Duration?: number;
  Width?: number;
  Height?: number;
  FilePath: string;
  Thumbnail: string;
  MessageIdx: number;
  PartIdx: number;
}

export interface A2AAgentInfo {
  name: string;
  description: string;
  skills?: A2ASkillInfo[];
}

export interface A2ASkillInfo {
  id: string;
  name: string;
  description?: string;
  tags?: string[];
}

export interface SelfPlayRoleInfo {
  provider?: string;
  model?: string;
  region?: string;
}

// === App State ===

// LiveMessage is what ActiveRun.messages stores: the SSE index plus whatever
// Message fields are known so far. The thin `message.created` event
// populates only role/content/tool_calls/tool_result; the later
// `message.full` event upserts the same index with the rest (cost_info,
// meta, latency_ms, validations, parts, timestamp, finish_reason) once the
// backend has persisted the message — the same shape the historical/REST
// `adaptMessage` path already renders.
export interface LiveMessage extends Message {
  index: number;
}

export interface ActiveRun {
  runId: string;
  scenario: string;
  provider: string;
  region: string;
  startTime: string;
  turnIndex: number;
  messages: LiveMessage[];
  costs: { inputTokens: number; outputTokens: number; totalCost: number };
  status: "running" | "completed" | "failed";
  duration?: number;
  error?: string;
}

export interface ArenaState {
  connected: boolean;
  runs: Record<string, ActiveRun>;
  completedRunIds: string[];
  totalCost: number;
  totalTokens: number;
  logs: LogEntry[];
}

export interface LogEntry {
  timestamp: string;
  level: "info" | "error";
  message: string;
  runId?: string;
}

// === Atlas Viewmodels (arenaView selectors) ===

export interface TrialCell {
  scenarioId: string;
  providerId: string;
  key: string; // `${scenarioId}:${providerId}`
  passRate: number; // 0-100
  passed: boolean; // passRate resolves to a pass (assertions all passed)
  best: boolean; // best provider in this scenario row
  costUsd: number; // total cost; 0 => rendered "free"
  latencyMs: number; // run duration in ms
  runId: string; // the RunResult.RunID backing this cell (latest)
  hasData: boolean; // false => empty cell (no run yet)
}

export interface TrialRow {
  scenarioId: string;
  label: string;
  cells: TrialCell[];
}

export interface TrialMatrix {
  providers: { id: string; label: string }[];
  rows: TrialRow[];
}

export interface Standing {
  rank: number;
  providerId: string;
  label: string;
  wins: number;
  leader: boolean;
}

export interface OverallGauge {
  passRate: number;
  passed: number;
  total: number;
  caption: string; // e.g. "13 / 20 passed"
}

export interface TranscriptMessage {
  role: string;
  idx: number;
  accent: string; // from roleAccent(role)
  bg: string; // color-mix accent 11%
  content?: string;
  meta?: string; // e.g. "$0.0069 · 820ms"
  tool?: { name: string; body: string };
  asserts?: { name: string; ok: boolean }[];
}
