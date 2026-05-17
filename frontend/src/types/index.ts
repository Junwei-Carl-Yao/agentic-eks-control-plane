// Mirrors backend Go DTOs in backend/internal/kubernetes/types.go. Field names
// must stay camelCase to match the json tags exactly.

export interface Deployment {
  name: string;
  namespace: string;
  replicas: number;
  availableReplicas: number;
  updatedReplicas: number;
  paused: boolean;
}

export interface Pod {
  name: string;
  namespace: string;
  phase: string;
  labels?: Record<string, string>;
}

export interface ClusterEvent {
  namespace: string;
  reason: string;
  message: string;
  type: string;
  time: string;
  object?: string;
}

export interface ServicePort {
  name?: string;
  port: number;
  targetPort?: string;
  protocol?: string;
  nodePort?: number;
}

export interface Service {
  name: string;
  namespace: string;
  type: string;
  clusterIP: string;
  ports?: ServicePort[];
}

export interface Ingress {
  name: string;
  namespace: string;
  class?: string;
  hosts?: string[];
}

export interface HorizontalPodAutoscaler {
  name: string;
  namespace: string;
  minReplicas: number;
  maxReplicas: number;
  currentReplicas: number;
  targetRef?: string;
}

export interface Namespace {
  name: string;
  phase?: string;
}

export interface Node {
  name: string;
}

export interface ReplicaSet {
  name: string;
  namespace: string;
  replicas: number;
  availableReplicas: number;
  revision?: number;
  owner?: string;
}

// Mirrors backend/internal/guardrails/enforcer.go Decision.
export interface Decision {
  allow: boolean;
  action: string;
  subject: string;
  reason?: string;
}

// Denial response body shape, returned with HTTP 403 by every guarded route.
export interface DenialResponse {
  error: string;
  decision: Decision;
}

// Chat transcript entry the agent runtime accepts. The runtime is stateless
// and only consumes user/assistant roles — tool events are kept locally for
// rendering but never sent back.
export type TranscriptRole = 'user' | 'assistant';

export interface TranscriptMessage {
  role: TranscriptRole;
  content: string;
}

// SSE frames emitted by POST /api/agent/chat. The `type` field discriminates
// the union; see Phase 4 wire contract.
export type AgentEvent =
  | { type: 'tool_call'; id: string; tool: string; input: Record<string, unknown> }
  | { type: 'tool_result'; id: string; ok: boolean; result: unknown; error: string | null }
  | { type: 'text'; delta: string }
  | { type: 'done' }
  | { type: 'error'; message: string };
