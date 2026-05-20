// Thin fetch wrappers around the backend HTTP routes. Each method maps 1:1
// to a tool exposed to the agent. The wrappers do not impose policy — the
// backend enforcer is the only chokepoint. They translate non-2xx responses
// into structured failures so the agent can surface denials verbatim.

export interface ToolFailure {
  error?: true;
  denied?: true;
  status: number;
  message?: string;
  reason?: string;
  decision?: unknown;
}

export type ToolResponse<Body> = Body | ToolFailure;

function isFailure(response: unknown): response is ToolFailure {
  return (
    typeof response === 'object' &&
    response !== null &&
    ('error' in (response as Record<string, unknown>) ||
      'denied' in (response as Record<string, unknown>))
  );
}

export class BackendClient {
  constructor(private readonly baseUrl: string) {}

  private buildQuery(params: Record<string, string | number | undefined>): string {
    const usable = Object.entries(params).filter(
      ([, value]) => value !== undefined && value !== '',
    );
    if (usable.length === 0) return '';
    const search = new URLSearchParams();
    for (const [key, value] of usable) {
      search.set(key, String(value));
    }
    return `?${search.toString()}`;
  }

  private async get<Body>(
    path: string,
    params: Record<string, string | number | undefined> = {},
  ): Promise<ToolResponse<Body>> {
    const url = `${this.baseUrl}${path}${this.buildQuery(params)}`;
    return this.dispatch<Body>(url, { method: 'GET' });
  }

  private async post<Body>(path: string, body: unknown): Promise<ToolResponse<Body>> {
    const url = `${this.baseUrl}${path}`;
    return this.dispatch<Body>(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
  }

  private async dispatch<Body>(url: string, init: RequestInit): Promise<ToolResponse<Body>> {
    let response: Response;
    try {
      response = await fetch(url, init);
    } catch (caught) {
      const reason = caught instanceof Error ? caught.message : String(caught);
      return { error: true, status: 0, message: `network error: ${reason}` };
    }

    const text = await response.text();
    let parsed: unknown = null;
    if (text.length > 0) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = { raw: text };
      }
    }

    if (response.status === 403) {
      const reason = extractString(parsed, 'error') ?? 'denied by backend guardrail';
      const decision = extractValue(parsed, 'decision');
      return { denied: true, status: 403, reason, decision };
    }

    if (!response.ok) {
      const message =
        extractString(parsed, 'error') ?? `backend returned status ${response.status}`;
      return { error: true, status: response.status, message };
    }

    return parsed as Body;
  }

  // Read tools

  health(): Promise<ToolResponse<unknown>> {
    return this.get<unknown>('/health');
  }

  clusterInfo() {
    return this.get<unknown>('/api/cluster/info');
  }

  clusterHealth() {
    return this.get<unknown>('/api/cluster/health');
  }

  listDeployments(namespace: string) {
    return this.get<unknown>('/api/cluster/deployments', { namespace });
  }

  getDeployment(namespace: string, name: string) {
    return this.get<unknown>(`/api/cluster/deployments/${encodeURIComponent(name)}`, { namespace });
  }

  listPods(namespace: string, labelSelector?: string) {
    return this.get<unknown>('/api/cluster/pods', { namespace, labelSelector });
  }

  listEvents(namespace: string) {
    return this.get<unknown>('/api/cluster/events', { namespace });
  }

  tailLogs(namespace: string, pod: string, container: string, lines: number) {
    return this.get<unknown>('/api/cluster/logs', { namespace, pod, container, lines });
  }

  listServices(namespace: string) {
    return this.get<unknown>('/api/cluster/services', { namespace });
  }

  listIngresses(namespace: string) {
    return this.get<unknown>('/api/cluster/ingresses', { namespace });
  }

  listHpas(namespace: string) {
    return this.get<unknown>('/api/cluster/hpas', { namespace });
  }

  listNamespaces() {
    return this.get<unknown>('/api/cluster/namespaces');
  }

  listNodes() {
    return this.get<unknown>('/api/cluster/nodes');
  }

  listReplicaSets(namespace: string) {
    return this.get<unknown>('/api/cluster/replicasets', { namespace });
  }

  // Write tools

  scale(namespace: string, name: string, replicas: number) {
    return this.post<unknown>('/api/operations/scale', { namespace, name, replicas });
  }

  rolloutRestart(namespace: string, name: string) {
    return this.post<unknown>('/api/operations/rollout-restart', { namespace, name });
  }

  pauseRollout(namespace: string, name: string) {
    return this.post<unknown>('/api/operations/pause-rollout', { namespace, name });
  }

  resumeRollout(namespace: string, name: string) {
    return this.post<unknown>('/api/operations/resume-rollout', { namespace, name });
  }

  rollback(namespace: string, name: string, revision: number) {
    return this.post<unknown>('/api/operations/rollback', { namespace, name, revision });
  }
}

function extractValue(parsed: unknown, key: string): unknown {
  if (parsed && typeof parsed === 'object' && key in parsed) {
    return (parsed as Record<string, unknown>)[key];
  }
  return undefined;
}

function extractString(parsed: unknown, key: string): string | undefined {
  const value = extractValue(parsed, key);
  return typeof value === 'string' ? value : undefined;
}

export { isFailure };
