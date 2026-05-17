import { useAgentChat } from '@/hooks/useAgentChat';

import { ChatInput } from './chat/ChatInput';
import { TranscriptList } from './chat/TranscriptList';

export function ChatPanel() {
  const { messages, isStreaming, send, stop, fatalError } = useAgentChat();

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex items-center justify-between border-b border-slate-800 bg-slate-900/40 px-4 py-3">
        <h2 className="text-base font-semibold text-slate-100">Agent</h2>
        <span className="text-xs text-slate-500">{isStreaming ? 'streaming…' : 'idle'}</span>
      </header>
      <div className="min-h-0 flex-1">
        <TranscriptList messages={messages} isStreaming={isStreaming} />
      </div>
      {fatalError && (
        <div className="border-t border-rose-900/60 bg-rose-950/40 px-3 py-1 text-xs text-rose-200">
          {fatalError}
        </div>
      )}
      <ChatInput isStreaming={isStreaming} onSend={send} onStop={stop} />
    </div>
  );
}
