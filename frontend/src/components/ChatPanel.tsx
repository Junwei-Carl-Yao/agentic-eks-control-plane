import { useAgentChat } from '@/hooks/useAgentChat';

import { ChatInput } from './chat/ChatInput';
import { TranscriptList } from './chat/TranscriptList';

export function ChatPanel() {
  const { messages, isStreaming, send, stop, fatalError } = useAgentChat();

  return (
    <div className="cp-root">
      <header className="cp-header">
        <div className="cp-header-l">
          <h2 className="cp-header-title">Agent</h2>
          <span className="cp-header-model">claude-opus-4.7</span>
        </div>
        {isStreaming && (
          <span className="cp-header-status">
            <span className="cp-status-dot streaming" />
            streaming…
          </span>
        )}
      </header>
      <TranscriptList
        messages={messages}
        isStreaming={isStreaming}
        onSuggestion={(text) => send(text)}
      />
      {fatalError && <div className="cp-error">{fatalError}</div>}
      <ChatInput isStreaming={isStreaming} onSend={send} onStop={stop} />
    </div>
  );
}
