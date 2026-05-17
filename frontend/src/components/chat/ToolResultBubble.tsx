interface ToolResultBubbleProps {
  ok: boolean;
  result: unknown;
  error: string | null;
}

// Render the enforcer denial reason prominently when the backend returns a
// guardrail decision payload — the Phase 4 invariant says we never silently
// drop a denial.
export function ToolResultBubble({ ok, result, error }: ToolResultBubbleProps) {
  const denial = extractDenial(result);
  const isDenied = !ok || denial !== null;
  const summary = formatSummary(result, error);

  return (
    <div className="flex justify-start">
      <div
        className={
          isDenied
            ? 'max-w-[90%] rounded border border-amber-700/60 bg-amber-950/40 px-2 py-1 text-[11px] text-amber-100'
            : 'max-w-[90%] rounded border border-emerald-800/60 bg-emerald-950/30 px-2 py-1 text-[11px] text-emerald-100'
        }
      >
        <div>
          <span className="font-semibold">{isDenied ? 'denied' : 'ok'}</span>
          {denial && (
            <>
              {' · '}
              <span className="font-mono">{denial.action}</span>
              {' · '}
              <span className="font-mono">{denial.subject}</span>
            </>
          )}
        </div>
        <div className="mt-0.5 break-words">{denial?.reason ?? error ?? summary}</div>
      </div>
    </div>
  );
}

interface DenialShape {
  action: string;
  subject: string;
  reason?: string;
}

function extractDenial(result: unknown): DenialShape | null {
  if (!result || typeof result !== 'object') {
    return null;
  }
  const record = result as Record<string, unknown>;
  const decision = record.decision;
  if (!decision || typeof decision !== 'object') {
    return null;
  }
  const decisionRecord = decision as Record<string, unknown>;
  if (decisionRecord.allow === true) {
    return null;
  }
  return {
    action: String(decisionRecord.action ?? ''),
    subject: String(decisionRecord.subject ?? ''),
    reason:
      typeof decisionRecord.reason === 'string'
        ? decisionRecord.reason
        : typeof record.error === 'string'
          ? record.error
          : undefined,
  };
}

function formatSummary(result: unknown, error: string | null): string {
  if (error) {
    return error;
  }
  if (result === null || result === undefined) {
    return '(no payload)';
  }
  const json = JSON.stringify(result);
  return json.length > 220 ? json.slice(0, 220) + '…' : json;
}
