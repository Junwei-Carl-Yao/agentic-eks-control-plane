// In-process mock of the backend HTTP API. It mirrors the route surface, the
// guardrail shape, and the response bodies the real backend returns. Every
// call is recorded so the eval harness can assert on tool selection.

import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { URL } from 'node:url';

export interface RecordedCall {
  method: string;
  path: string;
  query: Record<string, string>;
  body: unknown;
  status: number;
  responseBody: unknown;
}

export interface MockBackendOptions {
  // Mirror of backend guardrail policy. Defaults match production: only
  // api-smoke is allowed and the replica cap is 10.
  allowedNamespaces?: string[];
  maxReplicas?: number;
}

const DNS_1123 = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;

export interface MockBackendHandle {
  url: string;
  port: number;
  calls: RecordedCall[];
  reset(): void;
  stop(): Promise<void>;
}

export async function startMockBackend(
  options: MockBackendOptions = {},
): Promise<MockBackendHandle> {
  const allowedNamespaces = options.allowedNamespaces ?? ['api-smoke'];
  const maxReplicas = options.maxReplicas ?? 10;

  const calls: RecordedCall[] = [];

  const server = createServer((request, response) => {
    handleRequest(request, response, { allowedNamespaces, maxReplicas, calls }).catch((caught) => {
      const reason = caught instanceof Error ? caught.message : String(caught);
      response.statusCode = 500;
      response.setHeader('Content-Type', 'application/json');
      response.end(JSON.stringify({ error: reason }));
    });
  });

  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', () => resolve()));
  const address = server.address();
  if (typeof address === 'string' || address === null) {
    throw new Error('mock backend failed to bind to a port');
  }
  const port = address.port;

  return {
    url: `http://127.0.0.1:${port}`,
    port,
    calls,
    reset() {
      calls.length = 0;
    },
    stop() {
      return new Promise<void>((resolve, reject) => {
        server.close((err) => (err ? reject(err) : resolve()));
      });
    },
  };
}

interface HandleContext {
  allowedNamespaces: string[];
  maxReplicas: number;
  calls: RecordedCall[];
}

async function handleRequest(
  request: IncomingMessage,
  response: ServerResponse,
  context: HandleContext,
): Promise<void> {
  const url = new URL(request.url ?? '/', 'http://127.0.0.1');
  const path = url.pathname;
  const method = request.method ?? 'GET';
  const query: Record<string, string> = {};
  url.searchParams.forEach((value, key) => {
    query[key] = value;
  });

  let body: unknown = null;
  if (method !== 'GET' && method !== 'HEAD') {
    body = await readJson(request);
  }

  const result = route({ method, path, query, body, context });

  context.calls.push({
    method,
    path,
    query,
    body,
    status: result.status,
    responseBody: result.body,
  });

  response.statusCode = result.status;
  response.setHeader('Content-Type', 'application/json');
  response.end(JSON.stringify(result.body));
}

interface RouteArgs {
  method: string;
  path: string;
  query: Record<string, string>;
  body: unknown;
  context: HandleContext;
}

interface RouteResult {
  status: number;
  body: unknown;
}

function route(args: RouteArgs): RouteResult {
  const { method, path, query, body, context } = args;

  if (path === '/health' && method === 'GET') {
    return { status: 200, body: { status: 'ok' } };
  }

  if (method === 'GET' && path.startsWith('/api/cluster/')) {
    return handleClusterRead(path, query, context);
  }

  if (method === 'POST' && path.startsWith('/api/operations/')) {
    return handleOperation(path, body, context);
  }

  return { status: 404, body: { error: `route not found: ${method} ${path}` } };
}

function handleClusterRead(
  path: string,
  query: Record<string, string>,
  context: HandleContext,
): RouteResult {
  if (path === '/api/cluster/namespaces') {
    return {
      status: 200,
      body: context.allowedNamespaces.map((name) => ({ name })),
    };
  }
  if (path === '/api/cluster/nodes') {
    return {
      status: 200,
      body: [{ name: 'ip-10-0-0-1.ec2.internal' }, { name: 'ip-10-0-0-2.ec2.internal' }],
    };
  }

  const namespace = query['namespace'];
  if (!namespace) {
    return { status: 400, body: { error: 'namespace is required' } };
  }
  const denial = checkNamespace(namespace, context);
  if (denial) return denial;

  if (path === '/api/cluster/deployments') {
    return { status: 200, body: cannedDeployments(namespace) };
  }
  if (path.startsWith('/api/cluster/deployments/')) {
    const name = decodeURIComponent(path.slice('/api/cluster/deployments/'.length));
    return { status: 200, body: cannedDeployment(namespace, name) };
  }
  if (path === '/api/cluster/pods') {
    return { status: 200, body: cannedPods(namespace) };
  }
  if (path === '/api/cluster/events') {
    return { status: 200, body: [] };
  }
  if (path === '/api/cluster/logs') {
    return { status: 200, body: { logs: 'mock log line\n' } };
  }
  if (path === '/api/cluster/services') {
    return { status: 200, body: [] };
  }
  if (path === '/api/cluster/ingresses') {
    return { status: 200, body: [] };
  }
  if (path === '/api/cluster/hpas') {
    return { status: 200, body: [] };
  }
  if (path === '/api/cluster/replicasets') {
    return { status: 200, body: [] };
  }
  return { status: 404, body: { error: `route not found: ${path}` } };
}

function handleOperation(path: string, body: unknown, context: HandleContext): RouteResult {
  if (!body || typeof body !== 'object') {
    return { status: 400, body: { error: 'request body must be a JSON object' } };
  }
  const payload = body as Record<string, unknown>;
  const namespace = typeof payload.namespace === 'string' ? payload.namespace : '';
  const name = typeof payload.name === 'string' ? payload.name : '';

  if (!namespace) return { status: 400, body: { error: 'namespace is required' } };
  if (!name) return { status: 400, body: { error: 'name is required' } };

  if (!DNS_1123.test(namespace)) {
    return denyResponse(
      'invalid-namespace',
      `${namespace}/${name}`,
      `namespace ${namespace} is not a valid DNS-1123 label`,
    );
  }
  if (!DNS_1123.test(name)) {
    return denyResponse(
      actionFromPath(path),
      `${namespace}/${name}`,
      `name ${name} is not a valid DNS-1123 label`,
    );
  }

  const denial = checkNamespace(namespace, context);
  if (denial) {
    // Re-shape the deny so it carries the right action label for this op.
    return denyResponse(
      actionFromPath(path),
      `${namespace}/${name}`,
      `namespace ${namespace} is not on the allowed list`,
    );
  }

  if (path === '/api/operations/scale') {
    const replicas = typeof payload.replicas === 'number' ? payload.replicas : -1;
    if (!Number.isInteger(replicas) || replicas < 1) {
      return { status: 400, body: { error: 'replicas must be >= 1' } };
    }
    if (replicas > context.maxReplicas) {
      return denyResponse(
        'scale',
        `${namespace}/${name}`,
        `replicas ${replicas} exceeds max replicas ${context.maxReplicas}`,
      );
    }
    return allowResponse('scale', `${namespace}/${name}`);
  }
  if (path === '/api/operations/rollout-restart') {
    return allowResponse('rollout-restart', `${namespace}/${name}`);
  }
  if (path === '/api/operations/pause-rollout') {
    return allowResponse('pause-rollout', `${namespace}/${name}`);
  }
  if (path === '/api/operations/resume-rollout') {
    return allowResponse('resume-rollout', `${namespace}/${name}`);
  }
  if (path === '/api/operations/rollback') {
    const revision = typeof payload.revision === 'number' ? payload.revision : -1;
    if (!Number.isInteger(revision) || revision < 0) {
      return { status: 400, body: { error: 'revision must be >= 0 (0 means previous)' } };
    }
    return allowResponse('rollback', `${namespace}/${name}`);
  }
  return { status: 404, body: { error: `route not found: ${path}` } };
}

function actionFromPath(path: string): string {
  return path.replace('/api/operations/', '');
}

function checkNamespace(namespace: string, context: HandleContext): RouteResult | null {
  if (!DNS_1123.test(namespace)) {
    return denyResponse('read', namespace, `namespace ${namespace} is not a valid DNS-1123 label`);
  }
  if (!context.allowedNamespaces.includes(namespace)) {
    return denyResponse('read', namespace, `namespace ${namespace} is not on the allowed list`);
  }
  return null;
}

function denyResponse(action: string, subject: string, reason: string): RouteResult {
  return {
    status: 403,
    body: {
      error: reason,
      decision: { allow: false, action, subject, reason },
    },
  };
}

function allowResponse(action: string, subject: string): RouteResult {
  return {
    status: 200,
    body: {
      status: 'ok',
      decision: { allow: true, action, subject },
    },
  };
}

function cannedDeployments(namespace: string): unknown {
  return [
    { name: 'web', namespace, replicas: 2, readyReplicas: 2, availableReplicas: 2 },
    { name: 'api', namespace, replicas: 3, readyReplicas: 3, availableReplicas: 3 },
  ];
}

function cannedDeployment(namespace: string, name: string): unknown {
  return { name, namespace, replicas: 2, readyReplicas: 2, availableReplicas: 2 };
}

function cannedPods(namespace: string): unknown {
  return [
    { name: `${namespace}-web-aaa`, namespace, phase: 'Running' },
    { name: `${namespace}-api-bbb`, namespace, phase: 'Running' },
  ];
}

async function readJson(request: IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(chunk as Buffer);
  }
  if (chunks.length === 0) return null;
  const text = Buffer.concat(chunks).toString('utf-8');
  if (text.length === 0) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}
