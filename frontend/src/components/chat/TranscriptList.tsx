import { useEffect, useMemo, useRef } from 'react';

import type { ChatMessage } from '@/hooks/useAgentChat';

import { MessageBubble } from './MessageBubble';
import { ToolCallBubble, type GuardrailState } from './ToolCallBubble';

interface TranscriptListProps {
  messages: ChatMessage[];
  isStreaming: boolean;
  onSuggestion?: (text: string) => void;
}

const SUGGESTIONS = [
  'What pods are failing?',
  'Scale agent to 9 replicas',
  'Restart the frontend rollout',
];

export function TranscriptList({ messages, isStreaming, onSuggestion }: TranscriptListProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (container) {
      container.scrollTop = container.scrollHeight;
    }
  }, [messages]);

  const guardrailStateByCallId = useMemo(() => {
    const states = new Map<string, GuardrailState>();
    for (const message of messages) {
      if (message.kind === 'tool_result') {
        states.set(message.callId, message.ok ? 'allow' : 'deny');
      }
    }
    return states;
  }, [messages]);

  if (messages.length === 0) {
    return (
      <div ref={containerRef} className="cp-transcript">
        <div className="cp-empty">
          <div className="cp-empty-title">Ask the agent</div>
          <div className="cp-empty-sub">
            It can read cluster state and run guardrailed actions: scale, rollout restart,
            pause/resume, rollback.
          </div>
          {onSuggestion && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {SUGGESTIONS.map((suggestion) => (
                <button
                  key={suggestion}
                  type="button"
                  className="cp-suggestion cp-bubble cp-bubble-assistant"
                  onClick={() => onSuggestion(suggestion)}
                  style={{ alignSelf: 'stretch', cursor: 'pointer', textAlign: 'left' }}
                >
                  {suggestion}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div ref={containerRef} className="cp-transcript">
      {messages.map((message) => {
        switch (message.kind) {
          case 'user':
            return <MessageBubble key={message.id} role="user" content={message.content} />;
          case 'assistant':
            return (
              <MessageBubble
                key={message.id}
                role="assistant"
                content={message.content}
                pending={isStreaming && message.content.length === 0}
              />
            );
          case 'tool_call':
            return (
              <ToolCallBubble
                key={message.id}
                tool={message.tool}
                input={message.input}
                guardrailState={guardrailStateByCallId.get(message.callId) ?? 'pending'}
              />
            );
          case 'tool_result':
            return null;
          case 'error':
            return (
              <div key={message.id} className="cp-row cp-row-error">
                <div className="cp-error">{message.message}</div>
              </div>
            );
        }
      })}
    </div>
  );
}
