// Section B: Backend response handling.
//
// Tools must surface backend success bodies verbatim, surface 403 denials as
// structured failures (not throws), surface other 4xx/5xx as structured
// errors, and never invoke a route they don't wrap.

import { describe, expect, it, afterEach } from 'vitest';

import { BackendClient, isFailure, type ToolFailure } from '../../src/backendClient.js';
import { installMockFetch, type MockFetchHandle } from './testUtils.js';

describe('BackendClient response handling', () => {
  let mock: MockFetchHandle | undefined;

  afterEach(() => {
    mock?.restore();
    mock = undefined;
  });

  it('returns 200 JSON body verbatim', async () => {
    const body = [{ name: 'web', namespace: 'api-smoke', replicas: 2, readyReplicas: 2 }];
    mock = installMockFetch({ status: 200, body });
    const client = new BackendClient('http://backend.test');

    const result = await client.listDeployments('api-smoke');
    expect(result).toEqual(body);
    expect(isFailure(result)).toBe(false);
  });

  it('returns 200 JSON for mutation calls verbatim', async () => {
    const body = {
      status: 'ok',
      decision: { allow: true, action: 'scale', subject: 'api-smoke/web' },
    };
    mock = installMockFetch({ status: 200, body });
    const client = new BackendClient('http://backend.test');

    const result = await client.scale('api-smoke', 'web', 3);
    expect(result).toEqual(body);
  });

  it('surfaces 403 denials as structured failure (not a throw, not a silent retry)', async () => {
    const denialBody = {
      error: 'namespace app is not on the allowed list',
      decision: {
        allow: false,
        action: 'scale',
        subject: 'app/web',
        reason: 'namespace app is not on the allowed list',
      },
    };
    mock = installMockFetch({ status: 403, body: denialBody });
    const client = new BackendClient('http://backend.test');

    const result = await client.scale('app', 'web', 3);

    expect(isFailure(result)).toBe(true);
    const failure = result as ToolFailure;
    // The exact field names ("denied", "reason", "decision") are
    // implementation choices, but the spec requires the agent be able to
    // surface BOTH the denial fact and the reason. Verify both are present.
    expect(failure.denied).toBe(true);
    expect(failure.status).toBe(403);
    expect(failure.reason).toBe('namespace app is not on the allowed list');
    expect(failure.decision).toEqual(denialBody.decision);
    // The mock recorded one call — no silent retry.
    expect(mock!.recorded).toHaveLength(1);
  });

  it('surfaces non-403 4xx as structured error containing status and message', async () => {
    mock = installMockFetch({ status: 400, body: { error: 'namespace is required' } });
    const client = new BackendClient('http://backend.test');

    const result = await client.scale('', 'web', 3);

    expect(isFailure(result)).toBe(true);
    const failure = result as ToolFailure;
    expect(failure.error).toBe(true);
    expect(failure.status).toBe(400);
    expect(failure.message).toBe('namespace is required');
  });

  it('surfaces 500 as structured error (not a throw)', async () => {
    mock = installMockFetch({ status: 500, body: { error: 'kube api unavailable' } });
    const client = new BackendClient('http://backend.test');

    let threw = false;
    let result: unknown;
    try {
      result = await client.listPods('api-smoke');
    } catch {
      threw = true;
    }
    expect(threw).toBe(false);
    const failure = result as ToolFailure;
    expect(failure.status).toBe(500);
    expect(failure.error).toBe(true);
    expect(failure.message).toBe('kube api unavailable');
  });

  it('each tool method calls only the route it wraps (no fan-out)', async () => {
    mock = installMockFetch({ status: 200, body: {} });
    const client = new BackendClient('http://backend.test');

    interface Expectation {
      label: string;
      invoke: () => Promise<unknown>;
      expectedPathPrefix: string;
      expectedMethod: string;
    }

    const expectations: Expectation[] = [
      {
        label: 'listDeployments',
        invoke: () => client.listDeployments('api-smoke'),
        expectedPathPrefix: '/api/cluster/deployments',
        expectedMethod: 'GET',
      },
      {
        label: 'getDeployment',
        invoke: () => client.getDeployment('api-smoke', 'web'),
        expectedPathPrefix: '/api/cluster/deployments/web',
        expectedMethod: 'GET',
      },
      {
        label: 'listPods',
        invoke: () => client.listPods('api-smoke'),
        expectedPathPrefix: '/api/cluster/pods',
        expectedMethod: 'GET',
      },
      {
        label: 'listEvents',
        invoke: () => client.listEvents('api-smoke'),
        expectedPathPrefix: '/api/cluster/events',
        expectedMethod: 'GET',
      },
      {
        label: 'tailLogs',
        invoke: () => client.tailLogs('api-smoke', 'web-1', 'app', 25),
        expectedPathPrefix: '/api/cluster/logs',
        expectedMethod: 'GET',
      },
      {
        label: 'listServices',
        invoke: () => client.listServices('api-smoke'),
        expectedPathPrefix: '/api/cluster/services',
        expectedMethod: 'GET',
      },
      {
        label: 'listIngresses',
        invoke: () => client.listIngresses('api-smoke'),
        expectedPathPrefix: '/api/cluster/ingresses',
        expectedMethod: 'GET',
      },
      {
        label: 'listHpas',
        invoke: () => client.listHpas('api-smoke'),
        expectedPathPrefix: '/api/cluster/hpas',
        expectedMethod: 'GET',
      },
      {
        label: 'listNamespaces',
        invoke: () => client.listNamespaces(),
        expectedPathPrefix: '/api/cluster/namespaces',
        expectedMethod: 'GET',
      },
      {
        label: 'listNodes',
        invoke: () => client.listNodes(),
        expectedPathPrefix: '/api/cluster/nodes',
        expectedMethod: 'GET',
      },
      {
        label: 'listReplicaSets',
        invoke: () => client.listReplicaSets('api-smoke'),
        expectedPathPrefix: '/api/cluster/replicasets',
        expectedMethod: 'GET',
      },
      {
        label: 'scale',
        invoke: () => client.scale('api-smoke', 'web', 3),
        expectedPathPrefix: '/api/operations/scale',
        expectedMethod: 'POST',
      },
      {
        label: 'rolloutRestart',
        invoke: () => client.rolloutRestart('api-smoke', 'web'),
        expectedPathPrefix: '/api/operations/rollout-restart',
        expectedMethod: 'POST',
      },
      {
        label: 'pauseRollout',
        invoke: () => client.pauseRollout('api-smoke', 'web'),
        expectedPathPrefix: '/api/operations/pause-rollout',
        expectedMethod: 'POST',
      },
      {
        label: 'resumeRollout',
        invoke: () => client.resumeRollout('api-smoke', 'web'),
        expectedPathPrefix: '/api/operations/resume-rollout',
        expectedMethod: 'POST',
      },
      {
        label: 'rollback',
        invoke: () => client.rollback('api-smoke', 'web', 0),
        expectedPathPrefix: '/api/operations/rollback',
        expectedMethod: 'POST',
      },
      {
        label: 'health',
        invoke: () => client.health(),
        expectedPathPrefix: '/health',
        expectedMethod: 'GET',
      },
    ];

    for (const expectation of expectations) {
      mock!.recorded.length = 0;
      await expectation.invoke();
      expect(
        mock!.recorded.length,
        `${expectation.label} fired ${mock!.recorded.length} requests`,
      ).toBe(1);
      const call = mock!.recorded[0]!;
      expect(call.method, `${expectation.label} method`).toBe(expectation.expectedMethod);
      const pathOnly = new URL(call.url).pathname;
      expect(pathOnly, `${expectation.label} path`).toBe(expectation.expectedPathPrefix);
    }
  });

  it('ignores response body that is not JSON (wraps as raw)', async () => {
    // Edge case: backend returns plaintext on 200. The client must not throw.
    const original = globalThis.fetch;
    let threw = false;
    let result: unknown;
    try {
      globalThis.fetch = (async () =>
        new Response('hello', {
          status: 200,
          headers: { 'Content-Type': 'text/plain' },
        })) as typeof fetch;
      const client = new BackendClient('http://backend.test');
      result = await client.health();
    } catch {
      threw = true;
    } finally {
      globalThis.fetch = original;
    }
    expect(threw).toBe(false);
    expect(result).toEqual({ raw: 'hello' });
  });
});
