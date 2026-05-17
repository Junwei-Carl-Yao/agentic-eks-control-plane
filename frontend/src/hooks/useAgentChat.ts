import { useCallback, useEffect, useRef, useState } from 'react';

import { SseLineParser } from '@/sse/parseSse';
import type { AgentEvent, TranscriptMessage } from '@/types';

// ChatMessage is the union the UI renders. Only `user` and `assistant`
// entries are echoed back to the runtime; tool entries stay local.
export type ChatMessage =
  | { kind: 'user'; id: string; content: string }
  | { kind: 'assistant'; id: string; content: string }
  | {
      kind: 'tool_call';
      id: string;
      callId: string;
      tool: string;
      input: Record<string, unknown>;
    }
  | {
      kind: 'tool_result';
      id: string;
      callId: string;
      ok: boolean;
      result: unknown;
      error: string | null;
    }
  | { kind: 'error'; id: string; message: string };

export interface UseAgentChatResult {
  messages: ChatMessage[];
  isStreaming: boolean;
  send: (text: string) => void;
  stop: () => void;
  fatalError: string | null;
}

let nextId = 1;
function makeId(prefix: string): string {
  nextId += 1;
  return `${prefix}-${nextId}`;
}

function transcriptFromMessages(messages: ChatMessage[]): TranscriptMessage[] {
  const transcript: TranscriptMessage[] = [];
  for (const message of messages) {
    if (message.kind === 'user') {
      transcript.push({ role: 'user', content: message.content });
    } else if (message.kind === 'assistant' && message.content.length > 0) {
      transcript.push({ role: 'assistant', content: message.content });
    }
  }
  return transcript;
}

export function useAgentChat(): UseAgentChatResult {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [fatalError, setFatalError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const messagesRef = useRef<ChatMessage[]>([]);
  const currentAssistantIdRef = useRef<string | null>(null);

  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);

  useEffect(() => {
    return () => {
      abortRef.current?.abort();
    };
  }, []);

  const stop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setIsStreaming(false);
  }, []);

  const send = useCallback(
    (text: string) => {
      const trimmed = text.trim();
      if (trimmed.length === 0 || isStreaming) {
        return;
      }
      setFatalError(null);

      const userMessage: ChatMessage = {
        kind: 'user',
        id: makeId('user'),
        content: trimmed,
      };
      const assistantId = makeId('assistant');
      const assistantPlaceholder: ChatMessage = {
        kind: 'assistant',
        id: assistantId,
        content: '',
      };
      currentAssistantIdRef.current = assistantId;

      const priorTranscript = transcriptFromMessages(messagesRef.current);
      setMessages((prev) => [...prev, userMessage, assistantPlaceholder]);
      setIsStreaming(true);

      const controller = new AbortController();
      abortRef.current = controller;

      void streamChat({
        transcript: priorTranscript,
        message: trimmed,
        signal: controller.signal,
        onEvent: (event) => applyEvent(setMessages, currentAssistantIdRef, event),
        onError: (message) => {
          setMessages((prev) => [...prev, { kind: 'error', id: makeId('error'), message }]);
          setFatalError(message);
        },
        onClose: () => {
          if (abortRef.current === controller) {
            abortRef.current = null;
          }
          setIsStreaming(false);
        },
      });
    },
    [isStreaming],
  );

  return { messages, isStreaming, send, stop, fatalError };
}

// Each assistant text run owns its own bubble. When a tool_call lands we drop
// the active placeholder if it was still empty, then null out the active id so
// the next text frame creates a fresh assistant bubble below the tool trace.
// This keeps the rendered order faithful to the stream order — text before a
// tool stays above its bubble; text after stays below.
function applyEvent(
  setMessages: React.Dispatch<React.SetStateAction<ChatMessage[]>>,
  currentAssistantIdRef: { current: string | null },
  event: AgentEvent,
) {
  switch (event.type) {
    case 'text': {
      const existingId = currentAssistantIdRef.current;
      if (existingId === null) {
        const newId = makeId('assistant');
        currentAssistantIdRef.current = newId;
        setMessages((prev) => [...prev, { kind: 'assistant', id: newId, content: event.delta }]);
      } else {
        setMessages((prev) =>
          prev.map((message) =>
            message.kind === 'assistant' && message.id === existingId
              ? { ...message, content: message.content + event.delta }
              : message,
          ),
        );
      }
      break;
    }
    case 'tool_call': {
      const idToDropIfEmpty = currentAssistantIdRef.current;
      currentAssistantIdRef.current = null;
      setMessages((prev) => {
        const cleaned =
          idToDropIfEmpty !== null
            ? prev.filter(
                (message) =>
                  !(
                    message.kind === 'assistant' &&
                    message.id === idToDropIfEmpty &&
                    message.content.length === 0
                  ),
              )
            : prev;
        return [
          ...cleaned,
          {
            kind: 'tool_call',
            id: makeId('tool-call'),
            callId: event.id,
            tool: event.tool,
            input: event.input,
          },
        ];
      });
      break;
    }
    case 'tool_result': {
      setMessages((prev) => [
        ...prev,
        {
          kind: 'tool_result',
          id: makeId('tool-result'),
          callId: event.id,
          ok: event.ok,
          result: event.result,
          error: event.error,
        },
      ]);
      break;
    }
    case 'error': {
      setMessages((prev) => [
        ...prev,
        { kind: 'error', id: makeId('error'), message: event.message },
      ]);
      break;
    }
    case 'done': {
      const idToCheck = currentAssistantIdRef.current;
      if (idToCheck === null) break;
      setMessages((prev) =>
        prev.flatMap((message) => {
          if (
            message.kind === 'assistant' &&
            message.id === idToCheck &&
            message.content.length === 0
          ) {
            return [];
          }
          return [message];
        }),
      );
      break;
    }
  }
}

interface StreamChatOptions {
  transcript: TranscriptMessage[];
  message: string;
  signal: AbortSignal;
  onEvent: (event: AgentEvent) => void;
  onError: (message: string) => void;
  onClose: () => void;
}

async function streamChat(options: StreamChatOptions): Promise<void> {
  const { transcript, message, signal, onEvent, onError, onClose } = options;
  try {
    const response = await fetch('/api/agent/chat', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream',
      },
      body: JSON.stringify({ transcript, message }),
      signal,
    });
    if (!response.ok || !response.body) {
      onError(`agent runtime returned ${response.status} ${response.statusText}`);
      return;
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder('utf-8');
    const parser = new SseLineParser();
    for (;;) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }
      const frames = parser.feed(decoder.decode(value, { stream: true }));
      for (const frame of frames) {
        try {
          const parsed = JSON.parse(frame.data) as AgentEvent;
          onEvent(parsed);
          if (parsed.type === 'done') {
            return;
          }
        } catch {
          onError(`malformed SSE frame: ${frame.data}`);
        }
      }
    }
  } catch (error) {
    if (signal.aborted) {
      return;
    }
    onError(error instanceof Error ? error.message : String(error));
  } finally {
    onClose();
  }
}
