import { useEvents } from '@/hooks/useClusterQueries';

import { SectionShell } from './SectionShell';

interface EventsSectionProps {
  namespace: string;
}

export function EventsSection({ namespace }: EventsSectionProps) {
  const { data, isLoading, error } = useEvents(namespace);
  return (
    <SectionShell title="Events" count={data?.length} isLoading={isLoading} error={error}>
      {data && data.length === 0 && <div className="text-slate-500">No recent events.</div>}
      {data && data.length > 0 && (
        <ul className="space-y-1">
          {data.slice(0, 25).map((event, index) => (
            <li
              key={`${event.namespace}-${event.reason}-${event.time}-${index}`}
              className="rounded border border-slate-800/70 bg-slate-950/40 px-2 py-1"
            >
              <div className="flex items-center justify-between gap-2">
                <span className={event.type === 'Warning' ? 'text-amber-300' : 'text-emerald-300'}>
                  {event.reason}
                </span>
                <span className="text-[10px] text-slate-500">{formatTime(event.time)}</span>
              </div>
              {event.object && <div className="text-[11px] text-slate-500">{event.object}</div>}
              <div className="text-slate-300">{event.message}</div>
            </li>
          ))}
        </ul>
      )}
    </SectionShell>
  );
}

function formatTime(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleTimeString();
}
