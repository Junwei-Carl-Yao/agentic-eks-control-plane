// Section A: Tool surface conformance.
//
// The spec (implementation.md §4.2) lists the exact set of tools the agent
// runtime must expose, and the request shape each tool must produce against
// the backend. We assert both: the registered set equals the spec set
// (no missing, no extras), and each tool's HTTP call matches the documented
// route, method, and parameter shape.

import { describe, expect, it, afterEach } from 'vitest';

import { BackendClient } from '../../src/backendClient.js';
import { buildKubernetesMcpServer, MCP_SERVER_NAME, TOOL_NAMES } from '../../src/agents/tools.js';
import { installMockFetch, parseUrl, type MockFetchHandle } from './testUtils.js';

// The spec list (implementation.md §4.2). Tests fail if the runtime drifts.
const SPEC_TOOLS_READ = [
  'health_check',
  'list_deployments',
  'get_deployment',
  'list_pods',
  'list_events',
  'tail_logs',
  'list_services',
  'list_ingresses',
  'list_hpas',
  'list_namespaces',
  'list_nodes',
  'list_replicasets',
] as const;

const SPEC_TOOLS_WRITE = [
  'scale',
  'rollout_restart',
  'pause_rollout',
  'resume_rollout',
  'rollback',
] as const;

const SPEC_TOOLS_ALL = [...SPEC_TOOLS_READ, ...SPEC_TOOLS_WRITE];

interface ZodLikeSchema {
  safeParse(value: unknown): { success: boolean };
  def?: { shape?: Record<string, ZodLikeSchema> };
}

interface RegisteredTool {
  name: string;
  description: string;
  inputSchema: ZodLikeSchema;
  handler: (args: Record<string, unknown>, extra: unknown) => Promise<unknown>;
}

// fieldSchema reads the per-field Zod schema the SDK stored under the tool's
// inputSchema. The SDK builds a single ZodMiniObject from the {field: zod}
// map we pass, so the original per-field schemas live at .def.shape[name].
function fieldSchema(tool: RegisteredTool, fieldName: string): ZodLikeSchema {
  const shape = tool.inputSchema.def?.shape;
  if (!shape || !(fieldName in shape)) {
    throw new Error(`tool ${tool.name} has no field ${fieldName} in inputSchema`);
  }
  return shape[fieldName]!;
}

// fieldNames returns the input field names of a tool.
function fieldNames(tool: RegisteredTool): string[] {
  return Object.keys(tool.inputSchema.def?.shape ?? {});
}

function listRegisteredTools(): RegisteredTool[] {
  const server = buildKubernetesMcpServer(new BackendClient('http://backend.test'));
  // The MCP SDK keeps the live registry on the McpServer instance under
  // _registeredTools — what its ListTools handler iterates over. Reading
  // through it is the only way to assert "what the runtime actually exposes."
  const registry = (
    server.instance as unknown as { _registeredTools: Record<string, RegisteredTool> }
  )._registeredTools;
  return Object.entries(registry).map(([name, definition]) => ({ ...definition, name }));
}

describe('tool surface', () => {
  it('server name matches the documented MCP namespace', () => {
    expect(MCP_SERVER_NAME).toBe('kubernetes');
  });

  it('registers exactly the spec tool set', () => {
    const registered = listRegisteredTools()
      .map((tool) => tool.name)
      .sort();
    const expected = [...SPEC_TOOLS_ALL].sort();
    expect(registered).toEqual(expected);
  });

  it('exported TOOL_NAMES matches the registered tool set', () => {
    const registered = listRegisteredTools()
      .map((tool) => tool.name)
      .sort();
    const exported = [...TOOL_NAMES].sort();
    expect(exported).toEqual(registered);
  });

  it('contains no blocked tool families (delete_*, exec_*, read_secret*, *rbac*)', () => {
    const registered = listRegisteredTools().map((tool) => tool.name);
    for (const toolName of registered) {
      expect(toolName.startsWith('delete_'), `tool ${toolName} starts with delete_`).toBe(false);
      expect(toolName.startsWith('exec_'), `tool ${toolName} starts with exec_`).toBe(false);
      expect(toolName.startsWith('read_secret'), `tool ${toolName} starts with read_secret`).toBe(
        false,
      );
      expect(toolName.includes('rbac'), `tool ${toolName} mentions rbac`).toBe(false);
      expect(toolName.includes('secret'), `tool ${toolName} mentions secret`).toBe(false);
    }
  });
});

describe('tool -> backend route mapping', () => {
  let mock: MockFetchHandle;
  const tools = new Map<string, RegisteredTool>(
    listRegisteredTools().map((tool) => [tool.name, tool]),
  );

  function getTool(name: string): RegisteredTool {
    const tool = tools.get(name);
    if (!tool) throw new Error(`tool ${name} not registered`);
    return tool;
  }

  afterEach(() => {
    if (mock) mock.restore();
  });

  function setupOk(body: unknown = { ok: true }): void {
    mock = installMockFetch({ status: 200, body });
  }

  it('health_check -> GET /health (no query)', async () => {
    setupOk();
    await getTool('health_check').handler({}, {});
    expect(mock.recorded).toHaveLength(1);
    const call = mock.recorded[0]!;
    expect(call.method).toBe('GET');
    const parsed = parseUrl(call.url);
    expect(parsed.path).toBe('/health');
    expect(parsed.query).toEqual({});
  });

  it('list_deployments -> GET /api/cluster/deployments?namespace=...', async () => {
    setupOk([]);
    await getTool('list_deployments').handler({ namespace: 'api-smoke' }, {});
    const call = mock.recorded[0]!;
    expect(call.method).toBe('GET');
    const parsed = parseUrl(call.url);
    expect(parsed.path).toBe('/api/cluster/deployments');
    expect(parsed.query).toEqual({ namespace: 'api-smoke' });
  });

  it('get_deployment -> GET /api/cluster/deployments/{name}?namespace=...', async () => {
    setupOk({});
    await getTool('get_deployment').handler({ namespace: 'api-smoke', name: 'web' }, {});
    const call = mock.recorded[0]!;
    expect(call.method).toBe('GET');
    const parsed = parseUrl(call.url);
    expect(parsed.path).toBe('/api/cluster/deployments/web');
    expect(parsed.query).toEqual({ namespace: 'api-smoke' });
  });

  it('list_pods -> GET /api/cluster/pods with optional labelSelector', async () => {
    setupOk([]);
    await getTool('list_pods').handler({ namespace: 'api-smoke' }, {});
    const noSelector = parseUrl(mock.recorded[0]!.url);
    expect(noSelector.path).toBe('/api/cluster/pods');
    expect(noSelector.query).toEqual({ namespace: 'api-smoke' });

    await getTool('list_pods').handler({ namespace: 'api-smoke', labelSelector: 'app=web' }, {});
    const withSelector = parseUrl(mock.recorded[1]!.url);
    expect(withSelector.path).toBe('/api/cluster/pods');
    expect(withSelector.query).toEqual({ namespace: 'api-smoke', labelSelector: 'app=web' });
  });

  it('list_events -> GET /api/cluster/events?namespace=...', async () => {
    setupOk([]);
    await getTool('list_events').handler({ namespace: 'api-smoke' }, {});
    const parsed = parseUrl(mock.recorded[0]!.url);
    expect(parsed.path).toBe('/api/cluster/events');
    expect(parsed.query).toEqual({ namespace: 'api-smoke' });
  });

  it('tail_logs -> GET /api/cluster/logs with namespace, pod, container, lines', async () => {
    setupOk({ logs: '' });
    await getTool('tail_logs').handler(
      { namespace: 'api-smoke', pod: 'web-1', container: 'app', lines: 50 },
      {},
    );
    const call = mock.recorded[0]!;
    expect(call.method).toBe('GET');
    const parsed = parseUrl(call.url);
    expect(parsed.path).toBe('/api/cluster/logs');
    expect(parsed.query).toEqual({
      namespace: 'api-smoke',
      pod: 'web-1',
      container: 'app',
      lines: '50',
    });
  });

  it('tail_logs rejects non-positive lines via the input schema', () => {
    const tool = getTool('tail_logs');
    const linesSchema = fieldSchema(tool, 'lines');
    expect(linesSchema.safeParse(0).success).toBe(false);
    expect(linesSchema.safeParse(-5).success).toBe(false);
    expect(linesSchema.safeParse(1.5).success).toBe(false);
    expect(linesSchema.safeParse(50).success).toBe(true);
  });

  it('list_services / list_ingresses / list_hpas / list_replicasets all take only namespace', async () => {
    setupOk([]);
    const calls: { tool: string; expectedPath: string }[] = [
      { tool: 'list_services', expectedPath: '/api/cluster/services' },
      { tool: 'list_ingresses', expectedPath: '/api/cluster/ingresses' },
      { tool: 'list_hpas', expectedPath: '/api/cluster/hpas' },
      { tool: 'list_replicasets', expectedPath: '/api/cluster/replicasets' },
    ];
    for (const entry of calls) {
      await getTool(entry.tool).handler({ namespace: 'api-smoke' }, {});
    }
    for (let index = 0; index < calls.length; index += 1) {
      const parsed = parseUrl(mock.recorded[index]!.url);
      expect(parsed.path).toBe(calls[index]!.expectedPath);
      expect(parsed.query).toEqual({ namespace: 'api-smoke' });
    }
  });

  it('list_namespaces, list_nodes, health_check accept empty input and emit no query string', async () => {
    setupOk([]);
    const noArgTools = ['list_namespaces', 'list_nodes', 'health_check'];
    for (const toolName of noArgTools) {
      mock.recorded.length = 0;
      const tool = getTool(toolName);
      // schema must accept empty input — no required fields
      expect(fieldNames(tool), `${toolName} should expose no input fields`).toEqual([]);
      await tool.handler({}, {});
      const call = mock.recorded[0]!;
      const parsed = parseUrl(call.url);
      expect(parsed.query, `${toolName} sent unexpected query params`).toEqual({});
    }
    // re-check the namespace/nodes paths individually (handler ordering
    // matters; replay them in isolation).
    mock.recorded.length = 0;
    await getTool('list_namespaces').handler({}, {});
    expect(parseUrl(mock.recorded[0]!.url).path).toBe('/api/cluster/namespaces');
    await getTool('list_nodes').handler({}, {});
    expect(parseUrl(mock.recorded[1]!.url).path).toBe('/api/cluster/nodes');
  });

  it('scale -> POST /api/operations/scale with namespace, name, replicas', async () => {
    setupOk({ status: 'ok' });
    await getTool('scale').handler({ namespace: 'api-smoke', name: 'web', replicas: 3 }, {});
    const call = mock.recorded[0]!;
    expect(call.method).toBe('POST');
    expect(parseUrl(call.url).path).toBe('/api/operations/scale');
    expect(call.body).toEqual({ namespace: 'api-smoke', name: 'web', replicas: 3 });
    expect(call.headers['Content-Type']).toBe('application/json');
  });

  it('rollout_restart -> POST /api/operations/rollout-restart', async () => {
    setupOk({ status: 'ok' });
    await getTool('rollout_restart').handler({ namespace: 'api-smoke', name: 'api' }, {});
    const call = mock.recorded[0]!;
    expect(call.method).toBe('POST');
    expect(parseUrl(call.url).path).toBe('/api/operations/rollout-restart');
    expect(call.body).toEqual({ namespace: 'api-smoke', name: 'api' });
  });

  it('pause_rollout -> POST /api/operations/pause-rollout', async () => {
    setupOk({ status: 'ok' });
    await getTool('pause_rollout').handler({ namespace: 'api-smoke', name: 'api' }, {});
    expect(parseUrl(mock.recorded[0]!.url).path).toBe('/api/operations/pause-rollout');
    expect(mock.recorded[0]!.body).toEqual({ namespace: 'api-smoke', name: 'api' });
  });

  it('resume_rollout -> POST /api/operations/resume-rollout', async () => {
    setupOk({ status: 'ok' });
    await getTool('resume_rollout').handler({ namespace: 'api-smoke', name: 'api' }, {});
    expect(parseUrl(mock.recorded[0]!.url).path).toBe('/api/operations/resume-rollout');
    expect(mock.recorded[0]!.body).toEqual({ namespace: 'api-smoke', name: 'api' });
  });

  it('rollback sends revision: 0 (not omitted) when previous revision is requested', async () => {
    setupOk({ status: 'ok' });
    await getTool('rollback').handler({ namespace: 'api-smoke', name: 'api', revision: 0 }, {});
    const call = mock.recorded[0]!;
    expect(call.method).toBe('POST');
    expect(parseUrl(call.url).path).toBe('/api/operations/rollback');
    // The backend models say Revision >= 0 with 0 meaning previous; the body
    // MUST contain revision: 0 verbatim, never omit it.
    expect(call.body).toEqual({ namespace: 'api-smoke', name: 'api', revision: 0 });
    expect((call.body as Record<string, unknown>).revision).toBe(0);
  });

  it('rollback accepts positive revision values', async () => {
    setupOk({ status: 'ok' });
    await getTool('rollback').handler({ namespace: 'api-smoke', name: 'api', revision: 7 }, {});
    expect(mock.recorded[0]!.body).toEqual({ namespace: 'api-smoke', name: 'api', revision: 7 });
  });

  it('rollback rejects negative revisions at the input schema level', () => {
    const tool = getTool('rollback');
    const revisionSchema = fieldSchema(tool, 'revision');
    expect(revisionSchema.safeParse(-1).success).toBe(false);
    expect(revisionSchema.safeParse(0).success).toBe(true);
    expect(revisionSchema.safeParse(3).success).toBe(true);
  });
});
