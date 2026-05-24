import { useEffect } from 'react';

type IconName = 'agent' | 'shield' | 'tool' | 'cloud' | 'map' | 'code' | 'book' | 'person';
type ToolKind = 'read' | 'write';

interface Capability {
  icon: IconName;
  title: string;
  body: string;
}

interface StackLayer {
  area: string;
  tools: string[];
}

interface AgentTool {
  kind: ToolKind;
  name: string;
  desc: string;
  prompt: string;
}

const CAPABILITIES: Capability[] = [
  {
    icon: 'map',
    title: 'Dashboard',
    body: 'A live topology of the cluster — each availability zone, each node, and the pods on it grouped by deployment, with health and capacity inline.',
  },
  {
    icon: 'agent',
    title: 'Agent',
    body: 'A single Claude agent that knows your cluster. It maps natural language to a fixed set of read and mutation tools and runs them on your behalf.',
  },
  {
    icon: 'shield',
    title: 'Guardrail',
    body: 'A safety layer between the agent and the Kubernetes API. Every tool call passes through it, so safety lives in the backend instead of the prompt.',
  },
  {
    icon: 'cloud',
    title: 'Infrastructure',
    body: 'Terraform provisions the cluster, networking, node groups, and remote state. The same plan brings the environment up and tears it back down.',
  },
];

const STACK: StackLayer[] = [
  { area: 'Infrastructure', tools: ['Terraform', 'AWS EKS', 'Kubernetes', 'VPC', 'IAM', 'S3'] },
  { area: 'Backend', tools: ['Go', 'client-go'] },
  { area: 'Agent runtime', tools: ['Claude Agent SDK'] },
  { area: 'Frontend', tools: ['React', 'Vite', 'TypeScript', 'Claude Design'] },
  { area: 'Delivery', tools: ['Helm', 'ALB Controller'] },
];

const AGENT_TOOLS: AgentTool[] = [
  {
    kind: 'read',
    name: 'cluster_info',
    desc: 'Return cluster identity (name, region) and live healthy flag',
    prompt: 'Which cluster am I connected to?',
  },
  {
    kind: 'read',
    name: 'cluster_health',
    desc: 'Live apiserver healthy flag',
    prompt: 'Is the server healthy?',
  },
  {
    kind: 'read',
    name: 'list_namespaces',
    desc: 'List namespaces visible under the backend allowlist',
    prompt: 'Which namespaces can you see?',
  },
  {
    kind: 'read',
    name: 'list_nodes',
    desc: 'List node names and availability zones',
    prompt: 'How many nodes are in us-east-1b?',
  },
  {
    kind: 'read',
    name: 'list_deployments',
    desc: 'List deployments with replica state',
    prompt: 'Show me all deployments and their health',
  },
  {
    kind: 'read',
    name: 'get_deployment',
    desc: 'Fetch a single deployment by name',
    prompt: "What's the state of the agent deployment?",
  },
  {
    kind: 'read',
    name: 'list_replicasets',
    desc: 'List ReplicaSets in a namespace',
    prompt: 'Show ReplicaSets for the backend deployment',
  },
  {
    kind: 'read',
    name: 'list_pods',
    desc: 'List pods in a namespace, optional label filter',
    prompt: 'What pods are running in control-plane?',
  },
  {
    kind: 'read',
    name: 'list_services',
    desc: 'List services, their types and ports',
    prompt: 'How is the frontend deployment exposed?',
  },
  {
    kind: 'read',
    name: 'list_ingresses',
    desc: 'List ingresses in a namespace',
    prompt: "What's the public route to control-plane?",
  },
  {
    kind: 'read',
    name: 'list_hpas',
    desc: 'List HorizontalPodAutoscalers',
    prompt: 'Is anything autoscaling right now?',
  },
  {
    kind: 'read',
    name: 'list_events',
    desc: 'Read recent cluster events',
    prompt: 'Any warnings in the last 5 minutes?',
  },
  {
    kind: 'read',
    name: 'tail_logs',
    desc: 'Tail logs from a container in a pod',
    prompt: 'Get logs from worker-1b6a-hp9pk',
  },
  {
    kind: 'write',
    name: 'scale',
    desc: 'Set replica count within configured bounds',
    prompt: 'Scale agent to 8 replicas',
  },
  {
    kind: 'write',
    name: 'rollout_restart',
    desc: 'Trigger a rolling restart of a deployment',
    prompt: 'Restart the frontend deployment',
  },
  {
    kind: 'write',
    name: 'pause_rollout',
    desc: 'Pause an in-progress rollout',
    prompt: 'Pause the backend rollout while I investigate',
  },
  {
    kind: 'write',
    name: 'resume_rollout',
    desc: 'Resume a paused rollout',
    prompt: 'Resume the backend rollout',
  },
  {
    kind: 'write',
    name: 'rollback',
    desc: 'Roll back to a previous revision',
    prompt: 'Roll back agent to the previous version',
  },
];

const NAME = 'EKS Control Plane';
const TAGLINE = 'Agentic infrastructure operations, with guardrails';
const DESCRIPTION =
  'A control plane for Amazon EKS that turns natural-language requests into safe, constrained Kubernetes operations. The agent decides intent; the backend enforces what may actually be executed.';

interface PolicyRule {
  name: string;
  body: string;
}

const POLICY_RULES: PolicyRule[] = [
  {
    name: 'Namespace allowlist',
    body: 'Every namespace-scoped action targets a namespace on the allowlist. Today that is control-plane; everything else is denied at the enforcer.',
  },
  {
    name: 'DNS-1123 validation',
    body: 'Namespace, deployment, pod, and container names must be valid DNS-1123 labels — lowercase alphanumeric with dashes, 63 chars max.',
  },
  {
    name: 'Replica bounds',
    body: 'Scale requests are clamped to [2, 10]. The lower bound keeps a rolling restart from dropping pods to zero; the upper bound is a hardcoded ceiling.',
  },
  {
    name: 'Revision bounds',
    body: 'Rollback requires revision ≥ 0. Zero means the immediately previous revision; positive values target a specific historical one.',
  },
  {
    name: 'Rollback image floors',
    body: 'Per-deployment minimum image version. The target must carry a v<int> tag at or above the floor; digest pins, semver, and "latest" are rejected.',
  },
  {
    name: 'Audit trail',
    body: 'Every decision — allow or deny — is logged with action, subject, and reason. Denials also flow back in the API response so the UI can surface them.',
  },
];

interface InfoLink {
  icon: IconName;
  title: string;
  sub: string;
  href: string;
}

const INFO_LINKS: InfoLink[] = [
  {
    icon: 'code',
    title: 'GitHub repo',
    sub: 'github.com/Junwei-Carl-Yao/agentic-eks-control-plane',
    href: 'https://github.com/Junwei-Carl-Yao/agentic-eks-control-plane',
  },
  {
    icon: 'book',
    title: 'Architecture',
    sub: 'docs/architecture.md',
    href: 'https://github.com/Junwei-Carl-Yao/agentic-eks-control-plane/blob/master/docs/architecture.md',
  },
  {
    icon: 'shield',
    title: 'Safety',
    sub: 'docs/guardrails.md',
    href: 'https://github.com/Junwei-Carl-Yao/agentic-eks-control-plane/blob/master/docs/guardrails.md',
  },
  {
    icon: 'person',
    title: 'LinkedIn',
    sub: 'linkedin.com/in/junweiyao',
    href: 'https://www.linkedin.com/in/junweiyao/',
  },
];

const ICON_PATHS: Record<IconName, string> = {
  agent: 'M12 2 L20 7 V17 L12 22 L4 17 V7 Z M12 9 a3 3 0 1 1 0 6 a3 3 0 0 1 0 -6 Z',
  shield: 'M12 2 L20 5 V12 a8 8 0 0 1 -8 8 a8 8 0 0 1 -8 -8 V5 Z M9 12 l2 2 l4 -4',
  tool: 'M14 7 a4 4 0 1 0 -3 6.5 L4 21 l3 -7 a4 4 0 0 0 6 -3 L21 4 L18 4 L18 7 L14 7 Z',
  cloud: 'M6 16 a5 5 0 0 1 0 -10 a6 6 0 0 1 12 1 a4 4 0 0 1 0 9 Z',
  map: 'M4 4 h7 v7 h-7 z M13 4 h7 v7 h-7 z M4 13 h7 v7 h-7 z M13 13 h7 v7 h-7 z',
  code: 'M9 5 L3 12 L9 19 M15 5 L21 12 L15 19',
  book: 'M5 4 h11 a3 3 0 0 1 3 3 v13 a2 2 0 0 0 -2 -2 h-12 z M5 4 v14',
  person: 'M12 4 a4 4 0 1 0 0 8 a4 4 0 0 0 0 -8 z M4 21 v-2 a6 6 0 0 1 6 -6 h4 a6 6 0 0 1 6 6 v2',
};

function CapIcon({ name, size = 18 }: { name: IconName; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinejoin="round"
      strokeLinecap="round"
    >
      <path d={ICON_PATHS[name]} />
    </svg>
  );
}

interface AboutModalProps {
  onClose: () => void;
}

export function AboutModal({ onClose }: AboutModalProps) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div className="ab-overlay" onClick={onClose}>
      <div
        className="ab-modal"
        style={{ maxWidth: 800 }}
        onClick={(event) => event.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="about-title"
      >
        <button type="button" className="ab-close" onClick={onClose} aria-label="Close">
          ✕
        </button>

        <div className="c4">
          <div className="c4-hero">
            <div className="c4-hero-bg" />
            <div className="c4-hero-content">
              <div className="c4-eyebrow">About this project</div>
              <h1 id="about-title" className="c4-title">
                {NAME}
              </h1>
              <p className="c4-tagline">{TAGLINE}</p>
              <p className="c4-desc">{DESCRIPTION}</p>
            </div>
          </div>

          <section className="c4-section">
            <div className="c4-section-h">
              <h2 className="c4-h2">Capabilities</h2>
            </div>
            <div className="c4-grid">
              {CAPABILITIES.map((capability) => (
                <div key={capability.title} className="c4-card">
                  <div className="c4-card-icon">
                    <CapIcon name={capability.icon} size={20} />
                  </div>
                  <div className="c4-card-title">{capability.title}</div>
                  <div className="c4-card-body">{capability.body}</div>
                </div>
              ))}
            </div>
          </section>

          <section className="c4-section">
            <div className="c4-section-h">
              <h2 className="c4-h2">Tech stack</h2>
            </div>
            <div className="c4-stack">
              {STACK.map((layer, index) => (
                <div key={layer.area} className="c4-stack-layer">
                  <div className="c4-stack-l">
                    <span className="c4-stack-idx">{String(index + 1).padStart(2, '0')}</span>
                    <span className="c4-stack-area">{layer.area}</span>
                  </div>
                  <div className="c4-stack-r">
                    {layer.tools.map((toolName) => (
                      <span key={toolName} className="c4-stack-tool">
                        {toolName}
                      </span>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </section>

          <section className="c4-section">
            <div className="c4-section-h">
              <h2 className="c4-h2">Available tools</h2>
            </div>
            <div className="c4-tools">
              <div className="c4-tool-thead">
                <span>Tool</span>
                <span>Description</span>
                <span>Example prompt</span>
              </div>
              {AGENT_TOOLS.map((agentTool) => (
                <div key={agentTool.name} className="c4-tool-row">
                  <div className="c4-tool-c1">
                    <span className={`c4-tool-kind c4-tool-${agentTool.kind}`}>
                      {agentTool.kind}
                    </span>
                    <span className="c4-tool-name">{agentTool.name}</span>
                  </div>
                  <div className="c4-tool-c2">{agentTool.desc}</div>
                  <div className="c4-tool-c3">
                    <span className="c4-tool-prompt-quote">“</span>
                    {agentTool.prompt}
                    <span className="c4-tool-prompt-quote">”</span>
                  </div>
                </div>
              ))}
            </div>
          </section>

          <section className="c4-section">
            <div className="c4-section-h">
              <h2 className="c4-h2">Policy</h2>
            </div>
            <div className="c4-rules">
              {POLICY_RULES.map((rule, index) => (
                <div key={rule.name} className="c4-rule">
                  <div className="c4-rule-l">
                    <span className="c4-rule-idx">{String(index + 1).padStart(2, '0')}</span>
                    <span className="c4-rule-name">{rule.name}</span>
                  </div>
                  <div className="c4-rule-body">{rule.body}</div>
                </div>
              ))}
            </div>
          </section>

          <section className="c4-section">
            <div className="c4-section-h">
              <h2 className="c4-h2">More information</h2>
            </div>
            <div className="c4-links">
              {INFO_LINKS.map((link) => (
                <a
                  key={link.href}
                  href={link.href}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="c4-link"
                >
                  <div className="c4-link-icon">
                    <CapIcon name={link.icon} size={18} />
                  </div>
                  <div className="c4-link-text">
                    <div className="c4-link-title">{link.title}</div>
                    <div className="c4-link-sub">{link.sub}</div>
                  </div>
                  <span className="c4-link-arrow" aria-hidden>
                    ↗
                  </span>
                </a>
              ))}
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
