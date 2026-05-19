import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { AxiosError, AxiosHeaders } from 'axios';

import App from '@/App';
import { ZoneMap } from '@/components/ZoneMap';
import { apiClient } from '@/api/client';
import { renderWithClient } from '../testUtils';

// Spec-derived integration tests for the Zone Map + surrounding shell.
// Sources of truth (read first, then asserted against the live component):
//   - design-package/eks-cluster-topology/chats/chat1.md
//   - design-package/eks-cluster-topology/project/designs/zone-map.jsx
//   - design-package/eks-cluster-topology/project/Complete Page.html
//
// What the spec demands and these tests cover:
//   1. Pods on each node are GROUPED BY DEPLOYMENT, with the deployment NAME
//      shown beside its pod cluster ("Name of the deployment should appear
//      besides the pods still" — chat1.md).
//   2. Pod cells expose the pod's phase via a `!` glyph for CrashLoopBackOff
//      and `·` for Pending (zone-map.jsx PHASE_COLORS treatment).
//   3. Hovering a deployment row in the bottom Deployments panel marks its
//      pods focused — sibling deployments' pod clusters get the `dim` class.
//   4. Clicking a pod cell pins it in the Details panel; clicking again
//      unpins (chat1.md "Clicking a pod cell pins it in the Details panel.").
//   5. The Details panel empty state reads exactly:
//      "Hover or click a pod to inspect it." (zone-map.jsx PodDetail).
//   6. With no backend nodes, the Zone Map still shows 3 AZ columns
//      (us-east-1a/b/c) — the canonical regional fallback.
//   7. Theme persists via localStorage under the key `eks-theme` and is
//      restored on next render.

function makeAxiosError(status: number, data: unknown, message: string): AxiosError {
  const headers = new AxiosHeaders();
  return new AxiosError(message, String(status), { headers } as never, null, {
    status,
    statusText: '',
    headers,
    config: { headers } as never,
    data,
  });
}

interface RouterHandlers {
  [url: string]: (params?: Record<string, string>) => unknown | AxiosError;
}

function mockRouter(handlers: RouterHandlers) {
  return vi.spyOn(apiClient, 'get').mockImplementation((url: string, config?: unknown) => {
    const params = (config as { params?: Record<string, string> } | undefined)?.params;
    const handler = handlers[url];
    if (!handler) {
      return Promise.reject(makeAxiosError(500, { error: `unmocked ${url}` }, `unmocked ${url}`));
    }
    const result = handler(params);
    if (result instanceof AxiosError) return Promise.reject(result);
    return Promise.resolve({ data: result });
  });
}

beforeEach(() => {
  // Reset persisted UI state so theme tests aren't cross-polluted.
  localStorage.clear();
  vi.useRealTimers();
});

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  document.documentElement.removeAttribute('data-theme');
});

describe('ZoneMap — pod grouping by deployment (spec: chat1.md)', () => {
  it('renders one cluster per deployment on each node, with the deployment name beside the pods', async () => {
    // Two pods belonging to deployment "web", one belonging to deployment
    // "cache", all on the single returned node. Spec says each cluster shows
    // its deployment NAME beside the pod squares.
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': (params) => {
        if (params?.namespace === 'api-smoke') {
          return [
            {
              name: 'web',
              namespace: 'api-smoke',
              replicas: 2,
              availableReplicas: 2,
              updatedReplicas: 2,
              paused: false,
            },
            {
              name: 'cache',
              namespace: 'api-smoke',
              replicas: 1,
              availableReplicas: 1,
              updatedReplicas: 1,
              paused: false,
            },
          ];
        }
        return [];
      },
      '/api/cluster/pods': (params) => {
        if (params?.namespace === 'api-smoke') {
          return [
            {
              name: 'web-aaaaaaa',
              namespace: 'api-smoke',
              phase: 'Running',
              labels: { app: 'web' },
              nodeName: 'ip-10-0-1-14',
            },
            {
              name: 'web-bbbbbbb',
              namespace: 'api-smoke',
              phase: 'Running',
              labels: { app: 'web' },
              nodeName: 'ip-10-0-1-14',
            },
            {
              name: 'cache-ccccccc',
              namespace: 'api-smoke',
              phase: 'Running',
              labels: { app: 'cache' },
              nodeName: 'ip-10-0-1-14',
            },
          ];
        }
        return [];
      },
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(container.querySelectorAll('.zm-pod-cell').length).toBe(3);
    });
    // Two clusters: "web" and "cache". Their names live in elements with
    // the zm-pod-cluster-name class (per zone-map.jsx).
    const clusterNames = Array.from(container.querySelectorAll('.zm-pod-cluster-name')).map(
      (element) => element.textContent,
    );
    expect(clusterNames).toEqual(expect.arrayContaining(['web', 'cache']));
    // Exactly one swatch label per (deployment, node) — two on this single node.
    expect(clusterNames.length).toBe(2);
  });

  it('renders a `!` glyph on CrashLoopBackOff cells and `·` on Pending cells', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': (params) => {
        if (params?.namespace === 'api-smoke') {
          return [
            {
              name: 'worker-crash',
              namespace: 'api-smoke',
              phase: 'CrashLoopBackOff',
              labels: { app: 'worker' },
              nodeName: 'ip-10-0-1-14',
            },
            {
              name: 'web-pending',
              namespace: 'api-smoke',
              phase: 'Pending',
              labels: { app: 'web' },
              nodeName: 'ip-10-0-1-14',
            },
            {
              name: 'web-ok',
              namespace: 'api-smoke',
              phase: 'Running',
              labels: { app: 'web' },
              nodeName: 'ip-10-0-1-14',
            },
          ];
        }
        return [];
      },
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(container.querySelectorAll('.zm-pod-cell').length).toBe(3);
    });

    const cells = Array.from(container.querySelectorAll('.zm-pod-cell')) as HTMLElement[];
    const crashCell = cells.find((cell) => cell.getAttribute('title')?.includes('worker-crash'))!;
    const pendingCell = cells.find((cell) => cell.getAttribute('title')?.includes('web-pending'))!;
    const okCell = cells.find((cell) => cell.getAttribute('title')?.includes('web-ok'))!;

    expect(crashCell.querySelector('.zm-pod-bang')?.textContent).toBe('!');
    expect(pendingCell.querySelector('.zm-pod-bang.zm-pending')?.textContent).toBe('·');
    // Running pods have no glyph.
    expect(okCell.querySelector('.zm-pod-bang')).toBeNull();
  });
});

describe('ZoneMap — cross-panel interactions (spec: chat1.md)', () => {
  it('hovering a deployment row in the bottom panel dims sibling-deployment pod clusters', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': (params) => {
        if (params?.namespace === 'api-smoke') {
          return [
            {
              name: 'web',
              namespace: 'api-smoke',
              replicas: 1,
              availableReplicas: 1,
              updatedReplicas: 1,
              paused: false,
            },
            {
              name: 'cache',
              namespace: 'api-smoke',
              replicas: 1,
              availableReplicas: 1,
              updatedReplicas: 1,
              paused: false,
            },
          ];
        }
        return [];
      },
      '/api/cluster/pods': (params) => {
        if (params?.namespace === 'api-smoke') {
          return [
            {
              name: 'web-aaa',
              namespace: 'api-smoke',
              phase: 'Running',
              labels: { app: 'web' },
              nodeName: 'ip-10-0-1-14',
            },
            {
              name: 'cache-bbb',
              namespace: 'api-smoke',
              phase: 'Running',
              labels: { app: 'cache' },
              nodeName: 'ip-10-0-1-14',
            },
          ];
        }
        return [];
      },
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(container.querySelectorAll('.zm-pod-cell').length).toBe(2);
    });

    // Find the "web" row in the bottom Deployments panel.
    const depRows = Array.from(container.querySelectorAll('.zm-dep-row')) as HTMLElement[];
    const webRow = depRows.find((row) => within(row).queryByText('web'))!;
    expect(webRow).toBeTruthy();

    // No dim class anywhere before hover.
    const clustersBefore = container.querySelectorAll('.zm-pod-cluster.dim');
    expect(clustersBefore.length).toBe(0);

    fireEvent.mouseEnter(webRow);

    // After hovering "web", the OTHER deployment's pod cluster gets `dim`.
    await waitFor(() => {
      const dimmed = container.querySelectorAll('.zm-pod-cluster.dim');
      expect(dimmed.length).toBe(1);
    });
    const dimmedCluster = container.querySelector('.zm-pod-cluster.dim')!;
    // The dimmed cluster is the cache one, not the web one.
    expect(within(dimmedCluster as HTMLElement).queryByText('cache')).toBeTruthy();
    expect(within(dimmedCluster as HTMLElement).queryByText('web')).toBeNull();
  });

  it('clicking a pod cell pins it in the Details panel; clicking again unpins', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': (params) =>
        params?.namespace === 'api-smoke'
          ? [
              {
                name: 'web-aaa',
                namespace: 'api-smoke',
                phase: 'Running',
                labels: { app: 'web' },
                nodeName: 'ip-10-0-1-14',
              },
            ]
          : [],
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    // Empty state from PodDetail (spec: exact string).
    expect(await screen.findByText('Hover or click a pod to inspect it.')).toBeInTheDocument();

    await waitFor(() => {
      expect(container.querySelectorAll('.zm-pod-cell').length).toBe(1);
    });

    const cell = container.querySelector('.zm-pod-cell') as HTMLElement;

    // First click PINS — Details should now show the pod name as a heading-ish
    // element (.zm-detail-name) and the empty-state should be gone.
    fireEvent.click(cell);
    await waitFor(() => {
      expect(container.querySelector('.zm-detail-name')?.textContent).toBe('web-aaa');
    });
    expect(screen.queryByText('Hover or click a pod to inspect it.')).toBeNull();

    // Hover-out shouldn't lose the pin: the test simulates moving the mouse
    // off the cell to ensure the pin overrides hover state.
    fireEvent.mouseLeave(cell);
    expect(container.querySelector('.zm-detail-name')?.textContent).toBe('web-aaa');

    // Second click UNPINS. After mouseLeave + click, the pin clears AND hover
    // is gone, so the empty state returns.
    fireEvent.click(cell);
    fireEvent.mouseLeave(cell);
    await waitFor(() => {
      expect(screen.queryByText('Hover or click a pod to inspect it.')).toBeInTheDocument();
    });
  });
});

describe('ZoneMap — AZ fallback (spec: zone-map.jsx + chat1.md)', () => {
  it('renders 3 AZ columns (us-east-1a/b/c) when the backend returns zero nodes', async () => {
    mockRouter({
      '/api/cluster/nodes': () => [],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': () => [],
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    await waitFor(() => {
      const zones = container.querySelectorAll('.zm-zone');
      expect(zones.length).toBe(3);
    });
    const zoneNames = Array.from(container.querySelectorAll('.zm-zone-name')).map(
      (element) => element.textContent,
    );
    expect(zoneNames).toEqual(['us-east-1a', 'us-east-1b', 'us-east-1c']);
  });

  it('respects the zone projected on each node', async () => {
    // The Node DTO carries `zone` directly (resolved server-side from the
    // well-known topology label). ZoneMap groups nodes by that field, so a
    // labelled zone replaces the canonical AZ fallback entirely.
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-99-1', zone: 'us-west-2x' }],
      '/api/cluster/deployments': () => [],
      '/api/cluster/pods': () => [],
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    // Wait until the labelled node actually lands (initial render shows the
    // 3-AZ fallback because nodesQuery.data is undefined until the mock
    // resolves on a microtask).
    await waitFor(() => {
      const zoneNames = Array.from(container.querySelectorAll('.zm-zone-name')).map(
        (element) => element.textContent,
      );
      expect(zoneNames).toContain('us-west-2x');
    });
    const zoneNames = Array.from(container.querySelectorAll('.zm-zone-name')).map(
      (element) => element.textContent,
    );
    // Only the labelled zone — none of the canonical fallback letters.
    expect(zoneNames).not.toContain('us-east-1a');
    expect(zoneNames).not.toContain('us-east-1b');
    expect(zoneNames).not.toContain('us-east-1c');
  });
});

describe('App shell — theme persistence (spec: chat1.md "preference persists across reloads via localStorage")', () => {
  beforeEach(() => {
    // Network is irrelevant here; stub everything to empty arrays.
    vi.spyOn(apiClient, 'get').mockResolvedValue({ data: [] });
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('', { status: 200, headers: { 'Content-Type': 'text/event-stream' } }),
    );
  });

  it('writes the chosen theme to localStorage under key "eks-theme" and uses it on next mount', async () => {
    // Pre-seed: a saved "light" preference should be restored on render.
    localStorage.setItem('eks-theme', 'light');
    const first = renderWithClient(<App />);
    await waitFor(() => {
      expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    });
    // The button advertises the OPPOSITE theme it would switch to.
    expect(first.getByRole('button', { name: /Switch to dark mode/i })).toBeInTheDocument();

    // Toggle → persists "dark" to localStorage immediately.
    await act(async () => {
      first.getByRole('button', { name: /Switch to dark mode/i }).click();
    });
    await waitFor(() => {
      expect(localStorage.getItem('eks-theme')).toBe('dark');
    });

    // Unmount and remount: the new component should see the stored "dark"
    // value (simulating a reload).
    first.unmount();
    document.documentElement.removeAttribute('data-theme');
    const second = renderWithClient(<App />);
    await waitFor(() => {
      expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    });
    expect(second.getByRole('button', { name: /Switch to light mode/i })).toBeInTheDocument();
  });
});
