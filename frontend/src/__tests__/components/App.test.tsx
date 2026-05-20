import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, screen, waitFor } from '@testing-library/react';

import App from '@/App';
import { apiClient } from '@/api/client';
import { renderWithClient } from '../testUtils';

// Spec §5.3: "Two panes on a single page: cluster panel (left) + chat panel
// (right)." We verify the structural shell — the Zone Map cluster region and
// the agent chat region both render. Network is mocked to keep this fast.

beforeEach(() => {
  vi.spyOn(apiClient, 'get').mockImplementation((url: string) => {
    if (url === '/api/cluster/info') {
      return Promise.resolve({
        data: { name: 'eks-prod-us-east-1', region: 'us-east-1', healthy: true },
      });
    }
    if (url === '/api/cluster/health') {
      return Promise.resolve({ data: { healthy: true } });
    }
    return Promise.resolve({ data: [] });
  });
  // Stop fetch from actually firing if the chat panel kicks one off.
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response('', { status: 200, headers: { 'Content-Type': 'text/event-stream' } }),
  );
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('App', () => {
  it('renders the cluster region (Zone Map) and the agent chat pane on a single page', async () => {
    renderWithClient(<App />);

    await waitFor(() => {
      // Cluster region — ZoneMap exposes the cluster name as a heading-level
      // brand mark in its topbar.
      expect(screen.getByText(/eks-prod-us-east-1/i)).toBeInTheDocument();
    });
    // Chat region.
    expect(screen.getByRole('heading', { name: /Agent/i })).toBeInTheDocument();
    // App title in the header.
    expect(screen.getByText(/EKS Control Plane/)).toBeInTheDocument();

    // The two main regions live inside .app-main.
    const main = document.querySelector('.app-main');
    expect(main).not.toBeNull();
    expect(main!.querySelector('.app-cluster-region-l')).not.toBeNull();
    expect(main!.querySelector('.app-chat-region')).not.toBeNull();
  });

  it('exposes a theme toggle that flips data-theme on the document root', async () => {
    renderWithClient(<App />);
    const toggle = await screen.findByRole('button', { name: /Switch to .* mode/i });
    const initial = document.documentElement.getAttribute('data-theme');
    expect(['dark', 'light']).toContain(initial);

    await act(async () => {
      toggle.click();
    });
    await waitFor(() => {
      const next = document.documentElement.getAttribute('data-theme');
      expect(next).not.toBe(initial);
      expect(['dark', 'light']).toContain(next);
    });
  });

  it('does not render any router elements (single page only)', () => {
    renderWithClient(<App />);
    expect(document.querySelector('a[href]')).toBeNull();
    expect(document.body.innerHTML).not.toMatch(/#\//);
  });
});
