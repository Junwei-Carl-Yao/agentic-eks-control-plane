// Wire types for the POST /api/agent/chat SSE stream. The contract is owned
// by the frontend; do not extend it without coordinating with Phase 5.

export interface ChatTranscriptEntry {
  role: 'user' | 'assistant';
  content: string;
}

export interface ChatRequestBody {
  transcript: ChatTranscriptEntry[];
  message: string;
}

export type SseFrame =
  | { type: 'tool_call'; id: string; tool: string; input: Record<string, unknown> }
  | { type: 'tool_result'; id: string; ok: boolean; result: unknown; error: string | null }
  | { type: 'text'; delta: string }
  | { type: 'done' }
  | { type: 'error'; message: string };
