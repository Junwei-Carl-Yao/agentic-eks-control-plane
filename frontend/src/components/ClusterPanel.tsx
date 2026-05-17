import { useEffect, useState } from 'react';
import { useIsFetching } from '@tanstack/react-query';

import { DeploymentsSection } from './sections/DeploymentsSection';
import { EventsSection } from './sections/EventsSection';
import { NamespaceSelector } from './sections/NamespaceSelector';
import { NodesSection } from './sections/NodesSection';
import { PodsSection } from './sections/PodsSection';
import { ServicesSection } from './sections/ServicesSection';

const DEFAULT_NAMESPACE = 'api-smoke';

export function ClusterPanel() {
  const [namespace, setNamespace] = useState(DEFAULT_NAMESPACE);
  const [lastFetched, setLastFetched] = useState<Date | null>(null);
  const fetchingCount = useIsFetching();

  useEffect(() => {
    if (fetchingCount === 0) {
      setLastFetched(new Date());
    }
  }, [fetchingCount]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800 bg-slate-900/40 px-4 py-3">
        <div className="flex items-center gap-3">
          <h2 className="text-base font-semibold text-slate-100">Cluster</h2>
          <NamespaceSelector value={namespace} onChange={setNamespace} />
        </div>
        <div className="flex items-center gap-2 text-xs text-slate-400">
          <span
            className={
              fetchingCount > 0
                ? 'inline-block h-2 w-2 animate-pulse rounded-full bg-sky-400'
                : 'inline-block h-2 w-2 rounded-full bg-slate-700'
            }
            aria-hidden
          />
          <span>{fetchingCount > 0 ? 'refreshing…' : 'idle'}</span>
          {lastFetched && (
            <span className="text-slate-500">· last {lastFetched.toLocaleTimeString()}</span>
          )}
        </div>
      </header>
      <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 overflow-y-auto p-4 lg:grid-cols-2">
        <DeploymentsSection namespace={namespace} />
        <PodsSection namespace={namespace} />
        <ServicesSection namespace={namespace} />
        <NodesSection />
        <div className="lg:col-span-2">
          <EventsSection namespace={namespace} />
        </div>
      </div>
    </div>
  );
}
