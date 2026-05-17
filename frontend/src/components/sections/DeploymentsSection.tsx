import { useDeployments } from '@/hooks/useClusterQueries';

import { SectionShell } from './SectionShell';

interface DeploymentsSectionProps {
  namespace: string;
}

export function DeploymentsSection({ namespace }: DeploymentsSectionProps) {
  const { data, isLoading, error } = useDeployments(namespace);
  return (
    <SectionShell title="Deployments" count={data?.length} isLoading={isLoading} error={error}>
      {data && data.length === 0 && <Empty label="No deployments." />}
      {data && data.length > 0 && (
        <table className="w-full table-fixed border-collapse">
          <thead>
            <tr className="text-left text-[10px] uppercase tracking-wide text-slate-500">
              <th className="py-1 pr-2">Name</th>
              <th className="w-16 pr-2">Replicas</th>
              <th className="w-16 pr-2">Ready</th>
              <th className="w-20">Status</th>
            </tr>
          </thead>
          <tbody>
            {data.map((deployment) => (
              <tr
                key={`${deployment.namespace}/${deployment.name}`}
                className="border-t border-slate-800/60"
              >
                <td className="truncate py-1 pr-2 font-mono text-slate-200">{deployment.name}</td>
                <td className="pr-2 text-slate-300">{deployment.replicas}</td>
                <td className="pr-2 text-slate-300">{deployment.availableReplicas}</td>
                <td className="text-slate-400">{deployment.paused ? 'paused' : 'running'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </SectionShell>
  );
}

function Empty({ label }: { label: string }) {
  return <div className="text-slate-500">{label}</div>;
}
