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
    <form
      onSubmit={submit}
      className="flex items-end gap-2 border-t border-slate-800 bg-slate-900/60 p-3"
    >
      <textarea
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={onKeyDown}
        rows={2}
        disabled={isStreaming}
        placeholder={isStreaming ? 'Agent is responding…' : 'Ask the agent…'}
        className="min-h-[2.5rem] flex-1 resize-y rounded border border-slate-700 bg-slate-950 px-2 py-1.5 text-sm text-slate-100 placeholder:text-slate-500 focus:border-sky-500 focus:outline-none disabled:opacity-60"
      />
      {isStreaming ? (
        <button
          type="button"
          onClick={onStop}
          className="h-9 rounded bg-rose-600 px-3 text-sm font-medium text-white hover:bg-rose-500"
        >
          Stop
        </button>
      ) : (
        <button
          type="submit"
          disabled={draft.trim().length === 0}
          className="h-9 rounded bg-sky-600 px-3 text-sm font-medium text-white hover:bg-sky-500 disabled:cursor-not-allowed disabled:opacity-50"
        >
          Send
        </button>
      )}
    </form>
  );
}
