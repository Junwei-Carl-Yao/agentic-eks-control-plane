import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import { AxiosError, AxiosHeaders } from 'axios';

import { ZoneMap } from '@/components/ZoneMap';
import { apiClient } from '@/api/client';
import { renderWithClient } from '../testUtils';

// Spec §3.1 + §5.3: the cluster panel polls the read routes on the allowlisted
// namespace (api-smoke). A 403 from any single route must surface its
// guardrail reason without sinking the rest of the view.

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

describe('ZoneMap', () => {
  it('hits the cluster read routes for the allowlisted namespace', async () => {
    const get = mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/namespaces': () => [{ name: 'api-smoke' }],
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': () => [],
      '/api/cluster/events': () => [],
    });

    renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(get).toHaveBeenCalled();
    });

    const calls = get.mock.calls.map(([url, config]) => ({
      url: url as string,
      params: (config as { params?: Record<string, string> } | undefined)?.params,
    }));
    const urls = new Set(calls.map((call) => call.url));
    for (const expected of [
      '/api/cluster/nodes',
      '/api/cluster/deployments',
      '/api/cluster/pods',
      '/api/cluster/events',
    ]) {
      expect(urls.has(expected)).toBe(true);
    }

    const namespacesQueried = new Set(
      calls
        .filter((call) =>
          ['/api/cluster/deployments', '/api/cluster/pods', '/api/cluster/events'].includes(
            call.url,
          ),
        )
        .map((call) => call.params?.namespace),
    );
    expect(namespacesQueried).toEqual(new Set(['api-smoke']));
  });

  it('renders the cluster topbar with the discovered node count', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/nodes': () => [
        { name: 'ip-10-0-1-14' },
        { name: 'ip-10-0-2-31' },
        { name: 'ip-10-0-3-22' },
      ],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': () => [],
      '/api/cluster/events': () => [],
    });

    renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(screen.getByText(/eks-prod-us-east-1/i)).toBeInTheDocument();
    });
    // The "nodes" stat reflects the three returned nodes.
    const nodesLabel = await screen.findByText(/^nodes$/i);
    const statBlock = nodesLabel.parentElement!;
    expect(statBlock.textContent).toContain('3');
  });

  it('renders the topbar dot green and no disconnected label when /cluster/info reports healthy', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/nodes': () => [],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': () => [],
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);
    await waitFor(() => {
      expect(container.querySelector('.zm-cluster-dot-healthy')).not.toBeNull();
    });
    expect(container.querySelector('.zm-cluster-dot-unhealthy')).toBeNull();
    expect(container.querySelector('.zm-cluster-status-bad')).toBeNull();
  });

  it('renders the topbar dot red and a disconnected label when /cluster/info reports unhealthy', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: false,
      }),
      '/api/cluster/nodes': () => [],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': () => [],
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);
    await waitFor(() => {
      expect(container.querySelector('.zm-cluster-dot-unhealthy')).not.toBeNull();
    });
    expect(container.querySelector('.zm-cluster-dot-healthy')).toBeNull();
    const badge = container.querySelector('.zm-cluster-status-bad');
    expect(badge?.textContent?.toLowerCase()).toContain('disconnected');
  });

  it('surfaces a per-section denial reason without dropping the rest of the view', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': (params) =>
        params?.namespace === 'api-smoke'
          ? [
              {
                name: 'web',
                namespace: 'api-smoke',
                replicas: 2,
                availableReplicas: 2,
                updatedReplicas: 2,
                paused: false,
              },
            ]
          : [],
      '/api/cluster/pods': (params) =>
        params?.namespace === 'api-smoke'
          ? denialError('namespace foo is not on the allowed list')
          : [],
      '/api/cluster/events': () => [],
    });

    renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(screen.getByText(/namespace foo is not on the allowed list/i)).toBeInTheDocument();
    });
    // The deployments panel still renders even though pods failed.
    expect(screen.getByText('web')).toBeInTheDocument();
  });
});
