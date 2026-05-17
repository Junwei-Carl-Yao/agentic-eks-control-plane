import { useServices } from '@/hooks/useClusterQueries';

import { SectionShell } from './SectionShell';

interface ServicesSectionProps {
  namespace: string;
}

export function ServicesSection({ namespace }: ServicesSectionProps) {
  const { data, isLoading, error } = useServices(namespace);
  return (
    <SectionShell title="Services" count={data?.length} isLoading={isLoading} error={error}>
      {data && data.length === 0 && <div className="text-slate-500">No services.</div>}
      {data && data.length > 0 && (
        <ul className="divide-y divide-slate-800/60">
          {data.map((service) => (
            <li key={`${service.namespace}/${service.name}`} className="py-1">
              <div className="flex items-center justify-between gap-2">
                <span className="truncate font-mono text-slate-200">{service.name}</span>
                <span className="text-slate-400">{service.type}</span>
              </div>
              <div className="text-[11px] text-slate-500">
                {service.clusterIP}
                {service.ports && service.ports.length > 0 && (
                  <>
                    {' '}
                    ·{' '}
                    {service.ports
                      .map((port) => `${port.port}/${port.protocol ?? 'TCP'}`)
                      .join(', ')}
                  </>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </SectionShell>
  );
}
