import { useEffect, useRef } from 'react';

import type { ChatMessage } from '@/hooks/useAgentChat';

import { MessageBubble } from './MessageBubble';
import { ToolCallBubble } from './ToolCallBubble';

interface TranscriptListProps {
  messages: ChatMessage[];
  isStreaming: boolean;
}

export function TranscriptList({ messages, isStreaming }: TranscriptListProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (container) {
      container.scrollTop = container.scrollHeight;
    }
  }, [messages]);

  if (messages.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-slate-500">
        Ask the agent about cluster state or request a guardrailed action.
      </div>
    );
  }

  return (
    <div ref={containerRef} className="flex h-full flex-col gap-2 overflow-y-auto px-3 py-3">
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
            return <ToolCallBubble key={message.id} tool={message.tool} input={message.input} />;
          case 'tool_result':
            return null;
          case 'error':
            return (
              <div
                key={message.id}
                className="self-center rounded border border-rose-700/60 bg-rose-950/40 px-2 py-1 text-xs text-rose-200"
              >
                {message.message}
              </div>
            );
        }
      })}
    </div>
  );
}
