import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, waitFor, within } from '@testing-library/react';

import { ZoneMap } from '@/components/ZoneMap';
import { renderWithClient } from '../testUtils';
import { mockRouter } from '../mockRouter';

// Tests for the workload-image feature: the pod-detail panel must show one
// image row per container declared on the owning Deployment, sourced from the
// new `containers` field on the Deployment DTO. PodDetail is a non-exported
// helper inside ZoneMap.tsx, so we drive it via the public ZoneMap component
// with mocked query hooks rather than exporting internals just to test them.
//
// PodDetail renders each container as a KeyValue: label = container.name,
// value = container.image, with the `mono` class. The five standard fields
// (deployment, namespace, node, replicas, age) render unconditionally; image
// rows are added only when the owning Deployment carries containers and the
// pod's inferred deployment name matches a Deployment in the list.

beforeEach(() => {
  vi.useRealTimers();
});

afterEach(() => {
  vi.restoreAllMocks();
});

const STANDARD_FIELD_LABELS = ['deployment', 'namespace', 'node', 'replicas', 'age'];

function imageRowLabels(detailPanel: HTMLElement): string[] {
  const kvs = Array.from(detailPanel.querySelectorAll('.zm-kv')) as HTMLElement[];
  return kvs
    .map((kv) => kv.querySelector('.zm-kv-k')?.textContent ?? '')
    .filter((label) => !STANDARD_FIELD_LABELS.includes(label));
}

describe('ZoneMap PodDetail — container image rows', () => {
  it('renders one image row per container of the owning Deployment, with name as label and image as value', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/health': () => ({ healthy: true }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': (params) =>
        params?.namespace === 'control-plane'
          ? [
              {
                name: 'web',
                namespace: 'control-plane',
                replicas: 1,
                availableReplicas: 1,
                updatedReplicas: 1,
                paused: false,
                containers: [
                  { name: 'nginx', image: 'nginx:1.27' },
                  { name: 'istio-proxy', image: 'istio/proxyv2:1.20.0' },
                ],
              },
            ]
          : [],
      '/api/cluster/pods': (params) =>
        params?.namespace === 'control-plane'
          ? [
              {
                name: 'web-aaa',
                namespace: 'control-plane',
                phase: 'Running',
                labels: { app: 'web' },
                nodeName: 'ip-10-0-1-14',
                restartCount: 0,
                createdAt: new Date().toISOString(),
                cpuUsage: 0,
                memoryUsage: 0,
              },
            ]
          : [],
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(container.querySelectorAll('.zm-pod-cell').length).toBe(1);
    });

    fireEvent.click(container.querySelector('.zm-pod-cell') as HTMLElement);

    // Wait for the pin to take effect; the detail name is the load-bearing
    // sign the panel switched out of its empty state.
    await waitFor(() => {
      expect(container.querySelector('.zm-detail-name')?.textContent).toBe('web-aaa');
    });

    const detailPanel = container.querySelector('.zm-detail-name')?.parentElement as HTMLElement;
    expect(detailPanel).toBeTruthy();

    // Both container rows must be present, labelled by container.name with
    // value = container.image (raw, verbatim).
    const nginxRow = within(detailPanel).getByText('nginx').parentElement as HTMLElement;
    expect(nginxRow.querySelector('.zm-kv-v')?.textContent).toBe('nginx:1.27');

    const istioRow = within(detailPanel).getByText('istio-proxy').parentElement as HTMLElement;
    expect(istioRow.querySelector('.zm-kv-v')?.textContent).toBe('istio/proxyv2:1.20.0');

    // No extra image rows beyond the two declared.
    expect(imageRowLabels(detailPanel)).toEqual(expect.arrayContaining(['nginx', 'istio-proxy']));
    expect(imageRowLabels(detailPanel).length).toBe(2);
  });

  it('renders no image rows when the owning Deployment has no containers field', async () => {
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/health': () => ({ healthy: true }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': (params) =>
        params?.namespace === 'control-plane'
          ? [
              {
                name: 'web',
                namespace: 'control-plane',
                replicas: 1,
                availableReplicas: 1,
                updatedReplicas: 1,
                paused: false,
                // containers omitted entirely — same shape the backend
                // serializes when Spec.Template.Spec.Containers is empty
                // (omitempty drops the key).
              },
            ]
          : [],
      '/api/cluster/pods': (params) =>
        params?.namespace === 'control-plane'
          ? [
              {
                name: 'web-aaa',
                namespace: 'control-plane',
                phase: 'Running',
                labels: { app: 'web' },
                nodeName: 'ip-10-0-1-14',
                restartCount: 0,
                createdAt: new Date().toISOString(),
                cpuUsage: 0,
                memoryUsage: 0,
              },
            ]
          : [],
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(container.querySelectorAll('.zm-pod-cell').length).toBe(1);
    });
    fireEvent.click(container.querySelector('.zm-pod-cell') as HTMLElement);

    await waitFor(() => {
      expect(container.querySelector('.zm-detail-name')?.textContent).toBe('web-aaa');
    });

    const detailPanel = container.querySelector('.zm-detail-name')?.parentElement as HTMLElement;
    // The five standard fields still render — nothing crashed, nothing skipped.
    for (const label of STANDARD_FIELD_LABELS) {
      expect(within(detailPanel).getByText(label)).toBeInTheDocument();
    }
    // No extra image rows beyond those five.
    expect(imageRowLabels(detailPanel)).toEqual([]);
  });

  it("renders no image rows when the pod's inferred deployment cannot be found in the deployments list", async () => {
    // Pod's `app` label says "ghost" but no Deployment named "ghost" exists.
    // The component must still render the five standard fields and skip image
    // rows rather than crashing on `deployment?.containers`.
    mockRouter({
      '/api/cluster/info': () => ({
        name: 'eks-prod-us-east-1',
        region: 'us-east-1',
        healthy: true,
      }),
      '/api/cluster/health': () => ({ healthy: true }),
      '/api/cluster/nodes': () => [{ name: 'ip-10-0-1-14' }],
      '/api/cluster/deployments': (params) =>
        params?.namespace === 'control-plane'
          ? [
              {
                name: 'web',
                namespace: 'control-plane',
                replicas: 1,
                availableReplicas: 1,
                updatedReplicas: 1,
                paused: false,
                containers: [{ name: 'nginx', image: 'nginx:1.27' }],
              },
            ]
          : [],
      '/api/cluster/pods': (params) =>
        params?.namespace === 'control-plane'
          ? [
              {
                name: 'ghost-aaa',
                namespace: 'control-plane',
                phase: 'Running',
                labels: { app: 'ghost' },
                nodeName: 'ip-10-0-1-14',
                restartCount: 0,
                createdAt: new Date().toISOString(),
                cpuUsage: 0,
                memoryUsage: 0,
              },
            ]
          : [],
      '/api/cluster/events': () => [],
    });

    const { container } = renderWithClient(<ZoneMap />);

    await waitFor(() => {
      expect(container.querySelectorAll('.zm-pod-cell').length).toBe(1);
    });
    fireEvent.click(container.querySelector('.zm-pod-cell') as HTMLElement);

    await waitFor(() => {
      expect(container.querySelector('.zm-detail-name')?.textContent).toBe('ghost-aaa');
    });

    const detailPanel = container.querySelector('.zm-detail-name')?.parentElement as HTMLElement;
    for (const label of STANDARD_FIELD_LABELS) {
      expect(within(detailPanel).getByText(label)).toBeInTheDocument();
    }
    expect(imageRowLabels(detailPanel)).toEqual([]);
    // Crucially, the web Deployment's nginx image MUST NOT appear — that would
    // be a false attribution of an image to a pod that doesn't own it.
    expect(within(detailPanel).queryByText('nginx:1.27')).toBeNull();
  });
});
