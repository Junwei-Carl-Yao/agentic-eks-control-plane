import { useState, type FormEvent, type KeyboardEvent } from 'react';

interface ChatInputProps {
  isStreaming: boolean;
  onSend: (text: string) => void;
  onStop: () => void;
}

export function ChatInput({ isStreaming, onSend, onStop }: ChatInputProps) {
  const [draft, setDraft] = useState('');

  function submit(event: FormEvent) {
    event.preventDefault();
    if (isStreaming || draft.trim().length === 0) {
      return;
    }
    onSend(draft);
    setDraft('');
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      submit(event);
    }
  }

  return (
    <form onSubmit={submit} className="cp-input">
      <textarea
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={onKeyDown}
        rows={2}
        disabled={isStreaming}
        placeholder={isStreaming ? 'Agent is responding…' : 'Ask the agent…'}
        className="cp-textarea"
      />
      {isStreaming ? (
        <button type="button" onClick={onStop} className="cp-btn cp-btn-stop">
          Stop
        </button>
      ) : (
        <button type="submit" disabled={draft.trim().length === 0} className="cp-btn cp-btn-send">
          Send <span className="cp-btn-key">↵</span>
        </button>
      )}
    </form>
  );
}
