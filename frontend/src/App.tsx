import { ChatPanel } from '@/components/ChatPanel';
import { ClusterPanel } from '@/components/ClusterPanel';

export default function App() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-slate-950">
      <header className="border-b border-slate-800 bg-slate-900/70 px-4 py-2">
        <h1 className="text-sm font-semibold uppercase tracking-wider text-slate-300">
          EKS Control Plane
        </h1>
      </header>
      <div className="grid min-h-0 flex-1 grid-cols-12">
        <section className="col-span-12 min-h-0 border-r border-slate-800 lg:col-span-8">
          <ClusterPanel />
        </section>
        <section className="col-span-12 min-h-0 lg:col-span-4">
          <ChatPanel />
        </section>
      </div>
    </div>
  );
}
