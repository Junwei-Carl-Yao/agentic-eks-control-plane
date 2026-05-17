import { useNodes } from '@/hooks/useClusterQueries';

import { SectionShell } from './SectionShell';

export function NodesSection() {
  const { data, isLoading, error } = useNodes();
  return (
    <SectionShell title="Nodes" count={data?.length} isLoading={isLoading} error={error}>
      {data && data.length === 0 && <div className="text-slate-500">No nodes reported.</div>}
      {data && data.length > 0 && (
        <ul className="divide-y divide-slate-800/60">
          {data.map((node) => (
            <li key={node.name} className="py-1 font-mono text-slate-200">
              {node.name}
            </li>
          ))}
        </ul>
      )}
    </SectionShell>
  );
}
