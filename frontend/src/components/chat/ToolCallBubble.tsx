interface ToolCallBubbleProps {
  tool: string;
  input: Record<string, unknown>;
}

export function ToolCallBubble({ tool, input }: ToolCallBubbleProps) {
  const inputJson = JSON.stringify(input);
  const truncated = inputJson.length > 220 ? inputJson.slice(0, 220) + '…' : inputJson;
  return (
    <div className="flex justify-start">
      <div className="max-w-[90%] rounded border border-slate-700/60 bg-slate-900/70 px-2 py-1 text-[11px] text-slate-300">
        <div className="text-slate-400">
          <span className="text-sky-300">tool</span> ·{' '}
          <span className="font-mono text-slate-200">{tool}</span>
        </div>
        <pre className="mt-0.5 overflow-x-auto whitespace-pre-wrap break-words font-mono text-[10px] text-slate-400">
          {truncated}
        </pre>
      </div>
    </div>
  );
}
