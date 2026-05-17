import { usePods } from '@/hooks/useClusterQueries';

import { SectionShell } from './SectionShell';

interface PodsSectionProps {
  namespace: string;
}

const PHASE_COLORS: Record<string, string> = {
  Running: 'text-emerald-300',
  Pending: 'text-amber-300',
  Failed: 'text-rose-300',
  Succeeded: 'text-sky-300',
  Unknown: 'text-slate-400',
};

export function PodsSection({ namespace }: PodsSectionProps) {
  const { data, isLoading, error } = usePods(namespace);
  return (
    <SectionShell title="Pods" count={data?.length} isLoading={isLoading} error={error}>
      {data && data.length === 0 && <div className="text-slate-500">No pods.</div>}
      {data && data.length > 0 && (
        <ul className="divide-y divide-slate-800/60">
          {data.map((pod) => (
            <li
              key={`${pod.namespace}/${pod.name}`}
              className="flex items-center justify-between gap-2 py-1"
            >
              <span className="truncate font-mono text-slate-200">{pod.name}</span>
              <span className={PHASE_COLORS[pod.phase] ?? 'text-slate-400'}>{pod.phase}</span>
            </li>
          ))}
        </ul>
      )}
    </SectionShell>
  );
}
