import { useNamespaces } from '@/hooks/useClusterQueries';

interface NamespaceSelectorProps {
  value: string;
  onChange: (next: string) => void;
}

export function NamespaceSelector({ value, onChange }: NamespaceSelectorProps) {
  const { data, isLoading, error } = useNamespaces();

  return (
    <div className="flex items-center gap-2 text-sm">
      <label className="text-slate-400" htmlFor="namespace-select">
        Namespace
      </label>
      <select
        id="namespace-select"
        className="rounded border border-slate-700 bg-slate-900 px-2 py-1 text-slate-100 focus:border-sky-500 focus:outline-none"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {data === undefined && <option value={value}>{value}</option>}
        {data?.some((namespace) => namespace.name === value) === false && (
          <option value={value}>{value}</option>
        )}
        {data?.map((namespace) => (
          <option key={namespace.name} value={namespace.name}>
            {namespace.name}
          </option>
        ))}
      </select>
      {isLoading && <span className="text-xs text-slate-500">loading…</span>}
      {error && <span className="text-xs text-rose-400">namespaces unavailable</span>}
    </div>
  );
}
