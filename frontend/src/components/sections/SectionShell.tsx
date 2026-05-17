import type { ReactNode } from 'react';

import { ApiError } from '@/api/client';

interface SectionShellProps {
  title: string;
  count?: number;
  isLoading: boolean;
  error: unknown;
  children: ReactNode;
}

// SectionShell renders the loading shimmer, denial banner, and generic error
// state shared by every cluster panel section. A 403 from the backend carries
// a denial body; we render the enforcer's reason so the operator sees exactly
// what was rejected.
export function SectionShell({ title, count, isLoading, error, children }: SectionShellProps) {
  return (
    <section className="rounded-lg border border-slate-800 bg-slate-900/60">
      <header className="flex items-center justify-between border-b border-slate-800 px-3 py-2">
        <h3 className="text-sm font-semibold text-slate-200">{title}</h3>
        {count !== undefined && <span className="text-xs text-slate-500">{count}</span>}
      </header>
      <div className="px-3 py-2 text-xs">
        {error ? renderError(error) : isLoading ? <Shimmer /> : children}
      </div>
    </section>
  );
}

function renderError(error: unknown) {
  if (error instanceof ApiError && error.status === 403 && error.denial) {
    return (
      <div className="rounded border border-amber-700/50 bg-amber-950/40 px-2 py-1 text-amber-200">
        <div className="font-semibold">Denied by guardrail</div>
        <div className="mt-0.5 text-amber-300/90">
          {error.denial.decision.reason ?? error.denial.error}
        </div>
        <div className="mt-0.5 text-[10px] uppercase tracking-wide text-amber-400/70">
          {error.denial.decision.action} · {error.denial.decision.subject}
        </div>
      </div>
    );
  }
  const message = error instanceof Error ? error.message : String(error);
  return (
    <div className="rounded border border-rose-700/50 bg-rose-950/40 px-2 py-1 text-rose-200">
      {message}
    </div>
  );
}

function Shimmer() {
  return (
    <div className="space-y-1.5">
      <div className="h-3 animate-pulse rounded bg-slate-800" />
      <div className="h-3 w-5/6 animate-pulse rounded bg-slate-800" />
      <div className="h-3 w-2/3 animate-pulse rounded bg-slate-800" />
    </div>
  );
}
