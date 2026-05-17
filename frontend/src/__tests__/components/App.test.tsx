import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { screen, waitFor } from '@testing-library/react';

import App from '@/App';
import { apiClient } from '@/api/client';
import { renderWithClient } from '../testUtils';

// Spec §5.3: "Two panes on a single page: cluster panel (left) + chat panel
// (right)." We verify the structural shell — both regions render and the
// chat panel surfaces are wired up. Network is mocked to keep this fast.

beforeEach(() => {
  vi.spyOn(apiClient, 'get').mockResolvedValue({ data: [] });
  // Stop fetch from actually firing if the chat panel kicks one off.
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response('', { status: 200, headers: { 'Content-Type': 'text/event-stream' } }),
  );
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('App', () => {
  it('renders both the cluster pane and the chat pane on a single page', async () => {
    renderWithClient(<App />);
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /Cluster/i })).toBeInTheDocument();
    });
    expect(screen.getByRole('heading', { name: /Agent/i })).toBeInTheDocument();
    // App must lay out exactly TWO top-level sections (cluster pane + chat
    // pane). Inner sections inside the cluster panel use their own grid, so
    // we look only at the App's outer grid (12-column layout).
    const outerSections = document.querySelectorAll('div.grid-cols-12 > section');
    expect(outerSections.length).toBe(2);
  });

  it('does not render any router elements (single page only)', () => {
    renderWithClient(<App />);
    expect(document.querySelector('a[href]')).toBeNull();
    // No anchors with hash routes either.
    expect(document.body.innerHTML).not.toMatch(/#\//);
  });
});
