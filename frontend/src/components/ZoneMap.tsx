import { useMemo, useState } from 'react';

import {
  useClusterHealth,
  useClusterIdentity,
  useDeployments,
  useEvents,
  useNodes,
  usePods,
} from '@/hooks/useClusterQueries';
import type { ClusterEvent, Deployment, Node, Pod } from '@/types';

import { Splitter } from './Splitter';

const PRIMARY_NAMESPACE = 'control-plane';
const FALLBACK_REGION = 'us-east-1';
const UNSCHEDULED_BUCKET = 'unscheduled';

const PHASE_COLORS: Record<string, { foreground: string; background: string; ring: string }> = {
  Running: {
    foreground: '#34d399',
    background: 'rgba(52,211,153,0.18)',
    ring: 'rgba(52,211,153,0.55)',
  },
  Pending: {
    foreground: '#fbbf24',
    background: 'rgba(251,191,36,0.18)',
    ring: 'rgba(251,191,36,0.55)',
  },
  CrashLoopBackOff: {
    foreground: '#fb7185',
    background: 'rgba(251,113,133,0.18)',
    ring: 'rgba(251,113,133,0.55)',
  },
  Failed: {
    foreground: '#fb7185',
    background: 'rgba(251,113,133,0.18)',
    ring: 'rgba(251,113,133,0.55)',
  },
  Succeeded: {
    foreground: '#38bdf8',
    background: 'rgba(56,189,248,0.18)',
    ring: 'rgba(56,189,248,0.55)',
  },
  Unknown: {
    foreground: '#94a3b8',
    background: 'rgba(148,163,184,0.18)',
    ring: 'rgba(148,163,184,0.55)',
  },
  Terminating: {
    foreground: '#94a3b8',
    background: 'rgba(148,163,184,0.18)',
    ring: 'rgba(148,163,184,0.55)',
  },
};

interface PodWithDeployment extends Pod {
  deployment: string;
  node: string;
  age: string;
}

interface DeploymentWithStatus extends Deployment {
  status: 'healthy' | 'rolling' | 'degraded';
}

// Hash a name to a stable color. Designs use this so every deployment has
// its own consistent swatch across the topology — same swatch in the node
// card, the bottom legend, and the details panel.
//
// The earlier polynomial hash + raw `hash % 360` clustered similar short
// names onto neighboring hues (e.g. "backend" and "frontend" both landed in
// the pink band). FNV-1a gives better avalanche on short strings; the
// golden-angle multiplier (~137.508°) spreads neighboring hash values across
// the wheel; and small saturation/lightness buckets keyed off independent
// hash bits separate any two deployments that still collide on hue.
function depColor(name: string): string {
  let hash = 0x811c9dc5;
  for (let index = 0; index < name.length; index += 1) {
    hash ^= name.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  const hue = (hash * 137.508) % 360;
  const saturationBucket = (hash >>> 16) & 0x3;
  const lightnessBucket = (hash >>> 18) & 0x3;
  const saturation = 55 + saturationBucket * 10;
  const lightness = 60 + lightnessBucket * 6;
  return `hsl(${hue.toFixed(0)} ${saturation}% ${lightness}%)`;
}

// Identify the owning deployment for a pod. Standard EKS workloads carry an
// `app` or `app.kubernetes.io/name` label; we fall back to the first segment
// of the pod name (web-7d4b-abx1z → web) when labels are missing.
function podDeployment(pod: Pod): string {
  const labels = pod.labels;
  if (labels) {
    return (
      labels['app.kubernetes.io/name'] ??
      labels['app'] ??
      labels['k8s-app'] ??
      pod.name.split('-')[0]
    );
  }
  return pod.name.split('-')[0];
}

function deploymentStatus(deployment: Deployment): DeploymentWithStatus['status'] {
  if (deployment.availableReplicas === 0 && deployment.replicas > 0) return 'degraded';
  if (deployment.availableReplicas < deployment.replicas) {
    return deployment.paused ? 'degraded' : 'rolling';
  }
  if (deployment.updatedReplicas < deployment.replicas) return 'rolling';
  return 'healthy';
}

function formatRelativeAge(time: string): string {
  const parsed = new Date(time);
  if (Number.isNaN(parsed.getTime())) return '—';
  const seconds = Math.max(0, Math.floor((Date.now() - parsed.getTime()) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

interface ZoneMapDataState {
  nodes: Node[];
  pods: PodWithDeployment[];
  deployments: DeploymentWithStatus[];
  events: ClusterEvent[];
  zones: string[];
  podsByNode: Record<string, PodWithDeployment[]>;
  nodesByZone: Record<string, Node[]>;
  unscheduledPods: PodWithDeployment[];
  summary: { pods: number; healthyPods: number; namespaces: string[] };
  errors: string[];
}

export function ZoneMap() {
  const identityQuery = useClusterIdentity();
  const healthQuery = useClusterHealth();
  const nodesQuery = useNodes();
  const apiPodsQuery = usePods(PRIMARY_NAMESPACE);
  const apiDeploymentsQuery = useDeployments(PRIMARY_NAMESPACE);
  const apiEventsQuery = useEvents(PRIMARY_NAMESPACE);

  const region = identityQuery.data?.region ?? FALLBACK_REGION;

  const data: ZoneMapDataState = useMemo(() => {
    const rawNodes = nodesQuery.data ?? [];
    const rawPods = apiPodsQuery.data ?? [];
    const rawDeployments = apiDeploymentsQuery.data ?? [];
    const rawEvents = [...(apiEventsQuery.data ?? [])].sort(
      (a, b) => new Date(b.time).getTime() - new Date(a.time).getTime(),
    );

    const pods: PodWithDeployment[] = rawPods.map((pod) => ({
      ...pod,
      deployment: podDeployment(pod),
      node: pod.nodeName && pod.nodeName.length > 0 ? pod.nodeName : UNSCHEDULED_BUCKET,
      age: formatRelativeAge(pod.createdAt),
    }));

    const decoratedDeployments: DeploymentWithStatus[] = rawDeployments.map((deployment) => ({
      ...deployment,
      status: deploymentStatus(deployment),
    }));

    const zoneSet = new Set<string>();
    rawNodes.forEach((node) => {
      if (node.zone && node.zone.length > 0) zoneSet.add(node.zone);
    });
    const zones =
      zoneSet.size > 0 ? Array.from(zoneSet).sort() : [`${region}a`, `${region}b`, `${region}c`];

    const podsByNode: Record<string, PodWithDeployment[]> = {};
    const unscheduledPods: PodWithDeployment[] = [];
    pods.forEach((pod) => {
      if (pod.node === UNSCHEDULED_BUCKET) {
        unscheduledPods.push(pod);
        return;
      }
      (podsByNode[pod.node] ||= []).push(pod);
    });

    const nodesByZone: Record<string, Node[]> = {};
    zones.forEach((zone) => {
      nodesByZone[zone] = [];
    });
    rawNodes.forEach((node) => {
      const zone = node.zone && node.zone.length > 0 ? node.zone : zones[0];
      (nodesByZone[zone] ||= []).push(node);
    });

    const namespaces = Array.from(new Set(rawPods.map((pod) => pod.namespace))).sort();
    const summary = {
      pods: pods.length,
      healthyPods: pods.filter((pod) => pod.phase === 'Running').length,
      namespaces: namespaces.length > 0 ? namespaces : [PRIMARY_NAMESPACE],
    };

    const errors: string[] = [];
    const pushError = (label: string, error: unknown) => {
      if (error instanceof Error) errors.push(`${label}: ${error.message}`);
    };
    pushError('nodes', nodesQuery.error);
    pushError('pods', apiPodsQuery.error);
    pushError('deployments', apiDeploymentsQuery.error);
    pushError('events', apiEventsQuery.error);

    return {
      nodes: rawNodes,
      pods,
      deployments: decoratedDeployments,
      events: rawEvents,
      zones,
      podsByNode,
      nodesByZone,
      unscheduledPods,
      summary,
      errors,
    };
  }, [
    nodesQuery.data,
    nodesQuery.error,
    apiPodsQuery.data,
    apiPodsQuery.error,
    apiDeploymentsQuery.data,
    apiDeploymentsQuery.error,
    apiEventsQuery.data,
    apiEventsQuery.error,
    region,
  ]);

  const [hoverDeployment, setHoverDeployment] = useState<string | null>(null);
  const [hoverPod, setHoverPod] = useState<string | null>(null);
  const [selectedPod, setSelectedPod] = useState<string | null>(null);
  const [bottomHeight, setBottomHeight] = useState(320);

  const focusedPod = selectedPod ?? hoverPod;
  const focusedDeployment =
    hoverDeployment ??
    (focusedPod ? (data.pods.find((pod) => pod.name === focusedPod)?.deployment ?? null) : null);

  const podsHealthyTone =
    data.summary.healthyPods === data.summary.pods && data.summary.pods > 0 ? 'ok' : 'warn';

  const clusterName = identityQuery.data?.name ?? '—';
  const clusterRegion = identityQuery.data?.region ?? FALLBACK_REGION;
  // The probe has three observable states. A request error must collapse to
  // `unhealthy` — an unreachable backend is exactly what the red dot signals.
  // Leaving it on `pending` would silently hide outages behind a grey "still
  // connecting" indicator forever.
  const clusterStatus: 'pending' | 'healthy' | 'unhealthy' = healthQuery.error
    ? 'unhealthy'
    : healthQuery.data
      ? healthQuery.data.healthy
        ? 'healthy'
        : 'unhealthy'
      : 'pending';

  return (
    <div className="zm-root">
      <div className="zm-topbar">
        <div className="zm-cluster">
          <span
            className={`zm-cluster-dot zm-cluster-dot-${clusterStatus}`}
            title={
              clusterStatus === 'unhealthy'
                ? 'apiserver unreachable'
                : clusterStatus === 'pending'
                  ? 'connecting…'
                  : 'cluster healthy'
            }
          />
          <div>
            <div className="zm-cluster-name">{clusterName}</div>
            <div className="zm-cluster-sub">
              {clusterRegion}
              {clusterStatus === 'unhealthy' && (
                <span className="zm-cluster-status-bad"> · disconnected</span>
              )}
            </div>
          </div>
        </div>
        <div className="zm-stats">
          <Stat
            n={data.summary.healthyPods}
            of={data.summary.pods}
            label="pods healthy"
            tone={podsHealthyTone}
          />
          <Stat n={data.nodes.length} label="nodes" tone="ok" />
          <Stat n={data.deployments.length} label="deployments" tone="ok" />
          <Stat n={data.summary.namespaces.length} label="namespaces" tone="ok" />
        </div>
      </div>

      <div className="zm-body">
        <div className="zm-zones">
          {data.zones.map((zone) => (
            <div key={zone} className="zm-zone">
              <div className="zm-zone-head">
                <span className="zm-zone-tag">AZ</span>
                <span className="zm-zone-name">{zone}</span>
                <span className="zm-zone-count">{data.nodesByZone[zone]?.length ?? 0} nodes</span>
              </div>
              <div className="zm-zone-body">
                {(data.nodesByZone[zone] ?? []).map((node) => (
                  <NodeBlock
                    key={node.name}
                    node={node}
                    pods={data.podsByNode[node.name] ?? []}
                    focusedDeployment={focusedDeployment}
                    focusedPod={focusedPod}
                    onHoverPod={setHoverPod}
                    onClickPod={(podName) =>
                      setSelectedPod((current) => (current === podName ? null : podName))
                    }
                  />
                ))}
                {(data.nodesByZone[zone]?.length ?? 0) === 0 && (
                  <div className="zm-pod-empty">no nodes in this AZ</div>
                )}
              </div>
            </div>
          ))}
          {data.unscheduledPods.length > 0 && (
            <div className="zm-zone zm-zone-unscheduled">
              <div className="zm-zone-head">
                <span className="zm-zone-tag">!</span>
                <span className="zm-zone-name">unscheduled</span>
                <span className="zm-zone-count">{data.unscheduledPods.length} pods</span>
              </div>
              <div className="zm-zone-body">
                <PodGroupFlow
                  pods={data.unscheduledPods}
                  focusedDeployment={focusedDeployment}
                  focusedPod={focusedPod}
                  onHoverPod={setHoverPod}
                  onClickPod={(podName) =>
                    setSelectedPod((current) => (current === podName ? null : podName))
                  }
                  emptyLabel="no unscheduled pods"
                />
              </div>
            </div>
          )}
        </div>

        <Splitter
          axis="h"
          current={bottomHeight}
          setCurrent={setBottomHeight}
          min={120}
          max={500}
        />

        <div className="zm-bottom" style={{ height: `${bottomHeight}px` }}>
          <div className="zm-panel">
            <div className="zm-panel-head">Deployments</div>
            {data.deployments.length === 0 && (
              <div className="zm-detail-empty">No deployments.</div>
            )}
            <div className="zm-dep-list">
              {data.deployments.map((deployment) => (
                <button
                  key={`${deployment.namespace}/${deployment.name}`}
                  type="button"
                  className={
                    'zm-dep-row' + (focusedDeployment === deployment.name ? ' active' : '')
                  }
                  onMouseEnter={() => setHoverDeployment(deployment.name)}
                  onMouseLeave={() => setHoverDeployment(null)}
                  onClick={() => setSelectedPod(null)}
                >
                  <span
                    className="zm-dep-swatch"
                    style={{ backgroundColor: depColor(deployment.name) }}
                  />
                  <span className="zm-dep-name">{deployment.name}</span>
                  <span className="zm-dep-ns">{deployment.namespace}</span>
                  <span className={`zm-dep-status zm-${deployment.status}`}>
                    {deployment.availableReplicas}/{deployment.replicas}
                  </span>
                </button>
              ))}
            </div>
          </div>

          <div className="zm-panel">
            <div className="zm-panel-head">Details</div>
            <PodDetail podName={focusedPod} pods={data.pods} deployments={data.deployments} />
          </div>

          <div className="zm-panel">
            <div className="zm-panel-head">Recent events</div>
            <div className="zm-events">
              {data.events.length === 0 && <div className="zm-detail-empty">No recent events.</div>}
              {data.events.slice(0, 6).map((event, index) => (
                <div
                  key={`${event.namespace}-${event.reason}-${event.time}-${index}`}
                  className={`zm-event zm-event-${event.type.toLowerCase()}`}
                >
                  <div className="zm-event-row">
                    <span className="zm-event-reason">{event.reason}</span>
                    <span className="zm-event-age">{formatRelativeAge(event.time)}</span>
                  </div>
                  {event.object && <div className="zm-event-obj">{event.object}</div>}
                  <div className="zm-event-msg">{event.message}</div>
                </div>
              ))}
              {data.errors.map((message) => (
                <div key={message} className="zm-section-error">
                  {message}
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

interface StatProps {
  n: number;
  of?: number;
  label: string;
  tone: 'ok' | 'warn';
}

function Stat({ n, of, label, tone }: StatProps) {
  return (
    <div className="zm-stat">
      <div className={`zm-stat-num zm-stat-${tone}`}>
        {n}
        {of != null && <span className="zm-stat-of">/{of}</span>}
      </div>
      <div className="zm-stat-label">{label}</div>
    </div>
  );
}

interface NodeBlockProps {
  node: Node;
  pods: PodWithDeployment[];
  focusedDeployment: string | null;
  focusedPod: string | null;
  onHoverPod: (podName: string | null) => void;
  onClickPod: (podName: string) => void;
}

function NodeBlock({
  node,
  pods,
  focusedDeployment,
  focusedPod,
  onHoverPod,
  onClickPod,
}: NodeBlockProps) {
  // podCapacity == 0 means the node didn't report an allocatable pod count.
  // Render the capacity bar empty and elide the denominator instead of
  // pretending the node is 100% full.
  const capacity = node.podCapacity;
  const knownCapacity = capacity > 0;
  const usedFraction = knownCapacity ? pods.length / capacity : 0;
  const peakUtilization = Math.max(node.cpuUsage, node.memoryUsage, usedFraction);
  const utilizationTone = peakUtilization > 0.85 ? 'hot' : peakUtilization > 0.65 ? 'warm' : 'cool';
  const zoneSuffix = node.zone ? node.zone.slice(-2) : '—';

  return (
    <div className={`zm-node zm-node-${utilizationTone}`}>
      <div className="zm-node-head">
        <div className="zm-node-name">{node.name}</div>
        <div className="zm-node-meta">{node.instanceType ?? '—'}</div>
      </div>

      <div className="zm-node-bars">
        <MiniBar label="cpu" value={node.cpuUsage} />
        <MiniBar label="mem" value={node.memoryUsage} />
      </div>

      <PodGroupFlow
        pods={pods}
        focusedDeployment={focusedDeployment}
        focusedPod={focusedPod}
        onHoverPod={onHoverPod}
        onClickPod={onClickPod}
        emptyLabel="no pods scheduled"
      />

      <div className="zm-node-foot">
        <span className="zm-node-cap">
          <span className="zm-node-cap-track">
            <span
              className="zm-node-cap-fill"
              style={{ width: `${Math.min(1, usedFraction) * 100}%` }}
            />
          </span>
          <span className="zm-node-cap-text">
            {knownCapacity ? `${pods.length}/${capacity} pods` : `${pods.length} pods`}
          </span>
        </span>
        <span className="zm-node-az">{zoneSuffix}</span>
      </div>
    </div>
  );
}

interface PodGroupFlowProps {
  pods: PodWithDeployment[];
  focusedDeployment: string | null;
  focusedPod: string | null;
  onHoverPod: (podName: string | null) => void;
  onClickPod: (podName: string) => void;
  emptyLabel: string;
}

function PodGroupFlow({
  pods,
  focusedDeployment,
  focusedPod,
  onHoverPod,
  onClickPod,
  emptyLabel,
}: PodGroupFlowProps) {
  const groups: Record<string, PodWithDeployment[]> = {};
  pods.forEach((pod) => {
    (groups[pod.deployment] ||= []).push(pod);
  });
  const deploymentGroups = Object.entries(groups)
    .map(([deployment, deploymentPods]) => ({ deployment, pods: deploymentPods }))
    .sort((a, b) => b.pods.length - a.pods.length || a.deployment.localeCompare(b.deployment));

  return (
    <div className="zm-pod-flow">
      {deploymentGroups.length === 0 ? (
        <div className="zm-pod-empty">{emptyLabel}</div>
      ) : (
        deploymentGroups.map((group, groupIndex) => {
          const dimmed = focusedDeployment != null && group.deployment !== focusedDeployment;
          return (
            <span key={group.deployment} style={{ display: 'inline-flex', alignItems: 'center' }}>
              {groupIndex > 0 && <span className="zm-pod-divider" />}
              <span
                className={'zm-pod-cluster' + (dimmed ? ' dim' : '')}
                onMouseEnter={() => onHoverPod(group.pods[0].name)}
                onMouseLeave={() => onHoverPod(null)}
                title={`${group.deployment} · ${group.pods.length} pod${group.pods.length === 1 ? '' : 's'}`}
              >
                <span className="zm-pod-cluster-name" style={{ color: depColor(group.deployment) }}>
                  {group.deployment}
                </span>
                {group.pods.map((pod) => {
                  const phase = PHASE_COLORS[pod.phase] ?? PHASE_COLORS.Unknown;
                  const isFocused = focusedPod === pod.name;
                  return (
                    <button
                      key={pod.name}
                      type="button"
                      onMouseEnter={(event) => {
                        event.stopPropagation();
                        onHoverPod(pod.name);
                      }}
                      onClick={(event) => {
                        event.stopPropagation();
                        onClickPod(pod.name);
                      }}
                      className={'zm-pod-cell' + (isFocused ? ' focused' : '')}
                      style={{
                        backgroundColor: depColor(group.deployment),
                        boxShadow:
                          pod.phase !== 'Running'
                            ? `inset 0 0 0 2px ${phase.foreground}`
                            : undefined,
                      }}
                      title={`${pod.name} · ${pod.phase}`}
                      aria-label={`${pod.name} ${pod.phase}`}
                    >
                      {pod.phase === 'CrashLoopBackOff' && <span className="zm-pod-bang">!</span>}
                      {pod.phase === 'Pending' && <span className="zm-pod-bang zm-pending">·</span>}
                    </button>
                  );
                })}
              </span>
            </span>
          );
        })
      )}
    </div>
  );
}

function MiniBar({ label, value }: { label: string; value: number }) {
  const clamped = Math.max(0, Math.min(1, value));
  const color = clamped > 0.85 ? '#fb7185' : clamped > 0.65 ? '#fbbf24' : '#34d399';
  return (
    <div className="zm-mini">
      <span className="zm-mini-label">{label}</span>
      <span className="zm-mini-track">
        <span className="zm-mini-fill" style={{ width: `${clamped * 100}%`, background: color }} />
      </span>
      <span className="zm-mini-pct">{Math.round(clamped * 100)}%</span>
    </div>
  );
}

interface PodDetailProps {
  podName: string | null;
  pods: PodWithDeployment[];
  deployments: DeploymentWithStatus[];
}

function PodDetail({ podName, pods, deployments }: PodDetailProps) {
  if (!podName) {
    return <div className="zm-detail-empty">Hover or click a pod to inspect it.</div>;
  }
  const pod = pods.find((candidate) => candidate.name === podName);
  if (!pod) {
    return <div className="zm-detail-empty">Pod no longer present.</div>;
  }
  const deployment = deployments.find(
    (candidate) => candidate.name === pod.deployment && candidate.namespace === pod.namespace,
  );
  const phase = PHASE_COLORS[pod.phase] ?? PHASE_COLORS.Unknown;

  return (
    <div>
      <div className="zm-detail-head">
        <span
          className="zm-detail-phase"
          style={{
            background: phase.background,
            color: phase.foreground,
            borderColor: phase.ring,
          }}
        >
          {pod.phase}
        </span>
        <span className="zm-detail-restart">{pod.restartCount} restarts</span>
      </div>
      <div className="zm-detail-name">{pod.name}</div>
      <div className="zm-node-bars">
        <MiniBar label="cpu" value={pod.cpuUsage} />
        <MiniBar label="mem" value={pod.memoryUsage} />
      </div>
      <div className="zm-detail-grid">
        <KeyValue label="deployment" value={pod.deployment} swatch={depColor(pod.deployment)} />
        <KeyValue label="namespace" value={pod.namespace} />
        <KeyValue label="node" value={pod.node} mono />
        <KeyValue
          label="replicas"
          value={deployment ? `${deployment.availableReplicas}/${deployment.replicas}` : '—'}
        />
        <KeyValue label="age" value={pod.age} />
        {(deployment?.containers ?? []).map((container) => (
          <KeyValue key={container.name} label={container.name} value={container.image} mono />
        ))}
      </div>
    </div>
  );
}

interface KeyValueProps {
  label: string;
  value: string;
  mono?: boolean;
  swatch?: string;
}

function KeyValue({ label, value, mono, swatch }: KeyValueProps) {
  return (
    <div className="zm-kv">
      <div className="zm-kv-k">{label}</div>
      <div className={'zm-kv-v' + (mono ? ' mono' : '')}>
        {swatch && <span className="zm-kv-swatch" style={{ background: swatch }} />}
        {value}
      </div>
    </div>
  );
}
