import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import { AxiosError, AxiosHeaders } from 'axios';

import { ClusterPanel } from '@/components/ClusterPanel';
import { apiClient } from '@/api/client';
import { renderWithClient } from '../testUtils';

// We mock at the axios layer so the real clusterApi wrapper (and its ApiError
// translation of 403 denials) is exercised — that translation is what surfaces
// guardrail reasons in the UI per the §5.3 invariant. ApiError checks
// `error instanceof AxiosError`, so the rejection MUST be a real AxiosError
// instance — a plain Error with isAxiosError set is silently treated as a
// generic error and the denial banner never appears.

function makeAxiosError(status: number, data: unknown, message: string): AxiosError {
  const headers = new AxiosHeaders();
  const error = new AxiosError(message, String(status), { headers } as never, null, {
    status,
    statusText: '',
    headers,
    config: { headers } as never,
    data,
  });
  return error;
}

function denialError(message: string): AxiosError {
  return makeAxiosError(
    403,
    {
      error: message,
      decision: { allow: false, action: 'list_pods', subject: 'ns/foo', reason: message },
    },
    message,
  );
}

function genericError(status: number, message: string): AxiosError {
  return makeAxiosError(status, { error: message }, message);
}

// Per-route mock router. Returns whatever the test wired up for the URL.
function mockRouter(
  handlers: Record<string, (params?: Record<string, string>) => unknown | AxiosError>,
) {
  return vi.spyOn(apiClient, 'get').mockImplementation((url: string, config?: unknown) => {
    const params = (config as { params?: Record<string, string> } | undefined)?.params;
    const handler = handlers[url];
    if (!handler) {
      return Promise.reject(genericError(500, `unmocked URL ${url}`));
    }
    const result = handler(params);
    if (result instanceof AxiosError) {
      return Promise.reject(result);
    }
    return Promise.resolve({ data: result });
  });
}

beforeEach(() => {
  vi.useRealTimers();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ClusterPanel', () => {
  it('hits the correct backend URLs with the default api-smoke namespace', async () => {
    // Spec §3.1 + ClusterPanel default: api-smoke is the only allowlisted ns.
    const get = mockRouter({
      '/api/cluster/namespaces': () => [{ name: 'api-smoke' }],
      '/api/cluster/nodes': () => [{ name: 'node-1' }],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': () => [],
      '/api/cluster/services': () => [],
      '/api/cluster/events': () => [],
    });

    renderWithClient(<ClusterPanel />);

    await waitFor(() => {
      expect(get).toHaveBeenCalled();
    });

    const calls = get.mock.calls.map(([url, config]) => ({
      url: url as string,
      params: (config as { params?: Record<string, string> } | undefined)?.params,
    }));

    const urls = new Set(calls.map((call) => call.url));
    for (const expected of [
      '/api/cluster/namespaces',
      '/api/cluster/nodes',
      '/api/cluster/deployments',
      '/api/cluster/pods',
      '/api/cluster/services',
      '/api/cluster/events',
    ]) {
      expect(urls.has(expected)).toBe(true);
    }

    for (const call of calls) {
      if (
        call.url === '/api/cluster/deployments' ||
        call.url === '/api/cluster/pods' ||
        call.url === '/api/cluster/services' ||
        call.url === '/api/cluster/events'
      ) {
        expect(call.params?.namespace).toBe('api-smoke');
      }
    }

    for (const call of calls) {
      if (call.url === '/api/cluster/namespaces' || call.url === '/api/cluster/nodes') {
        expect(call.params).toBeUndefined();
      }
    }
  });

  it('renders the guardrail denial reason from a section endpoint that 403s', async () => {
    mockRouter({
      '/api/cluster/namespaces': () => [{ name: 'api-smoke' }],
      '/api/cluster/nodes': () => [{ name: 'node-1' }],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': () => denialError('namespace foo is not on the allowed list'),
      '/api/cluster/services': () => [],
      '/api/cluster/events': () => [],
    });

    renderWithClient(<ClusterPanel />);

    await waitFor(() => {
      expect(screen.getByText(/namespace foo is not on the allowed list/i)).toBeInTheDocument();
    });

    // The shell labels denials distinctly from generic errors.
    expect(screen.getByText(/Denied by guardrail/i)).toBeInTheDocument();
  });

  it('keeps other sections rendered when one section fetch fails', async () => {
    mockRouter({
      '/api/cluster/namespaces': () => [{ name: 'api-smoke' }],
      '/api/cluster/nodes': () => [{ name: 'node-zeta' }],
      '/api/cluster/deployments': () => [
        {
          name: 'web',
          namespace: 'api-smoke',
          replicas: 2,
          availableReplicas: 2,
          updatedReplicas: 2,
          paused: false,
        },
      ],
      '/api/cluster/pods': () => genericError(500, 'pods exploded'),
      '/api/cluster/services': () => [
        { name: 'web-svc', namespace: 'api-smoke', type: 'ClusterIP', clusterIP: '10.0.0.1' },
      ],
      '/api/cluster/events': () => [],
    });

    renderWithClient(<ClusterPanel />);

    // The failing section's error must show...
    await waitFor(() => {
      expect(screen.getByText(/pods exploded/i)).toBeInTheDocument();
    });
    // ...while the other sections still render their data.
    expect(screen.getByText('web')).toBeInTheDocument();
    expect(screen.getByText('web-svc')).toBeInTheDocument();
    expect(screen.getByText('node-zeta')).toBeInTheDocument();
  });
});
