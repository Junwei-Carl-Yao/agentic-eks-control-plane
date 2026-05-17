import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { ChatPanel } from '@/components/ChatPanel';
import { renderWithClient, makeSseResponse, makeSseResponseQueue } from '../testUtils';

// Spec §5.3 + fixed contract:
// - POST /api/agent/chat with body {transcript, message}
// - SSE wire: data: <json>\n\n; types: tool_call | tool_result | text | done | error
// - Heartbeats are SSE comments (`: ...`) and must be ignored
// - The frontend OWNS the transcript; it sends only role:user / role:assistant
//   entries (no tool events, no errors)
// - Input disabled while streaming; Stop affordance aborts via AbortController
// - Tool result payloads must not be rendered in the user transcript

interface CapturedRequest {
  url: string;
  method: string;
  body: unknown;
  signal: AbortSignal | null;
}

function captureFetch(handler: (request: CapturedRequest) => Response | Promise<Response>) {
  const requests: CapturedRequest[] = [];
  const spy = vi
    .spyOn(globalThis, 'fetch')
    .mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      const method = init?.method ?? 'GET';
      const rawBody = init?.body;
      const body =
        typeof rawBody === 'string' ? (JSON.parse(rawBody) as unknown) : (rawBody ?? null);
      const signal = (init?.signal as AbortSignal | undefined) ?? null;
      const captured: CapturedRequest = { url, method, body, signal };
      requests.push(captured);
      return Promise.resolve(handler(captured));
    });
  return { requests, spy };
}

beforeEach(() => {
  vi.useRealTimers();
});

afterEach(() => {
  vi.restoreAllMocks();
});

async function sendMessage(text: string) {
  const user = userEvent.setup();
  const textarea = screen.getByPlaceholderText(/Ask the agent…|Agent is responding…/);
  await user.type(textarea, text);
  await user.click(screen.getByRole('button', { name: /Send/i }));
}

describe('ChatPanel — wire contract', () => {
  it('POSTs to /api/agent/chat with {transcript, message}', async () => {
    const { requests } = captureFetch(() =>
      makeSseResponse(['data: {"type":"text","delta":"hi"}\n\n', 'data: {"type":"done"}\n\n']),
    );
    renderWithClient(<ChatPanel />);

    await sendMessage('list pods');

    await waitFor(() => {
      expect(requests).toHaveLength(1);
    });
    const request = requests[0];
    expect(request.url).toBe('/api/agent/chat');
    expect(request.method).toBe('POST');
    const body = request.body as { transcript: unknown[]; message: string };
    expect(body.message).toBe('list pods');
    // First turn: prior transcript is empty.
    expect(Array.isArray(body.transcript)).toBe(true);
    expect(body.transcript).toEqual([]);
  });

  it('omits tool_call / tool_result events from the transcript on subsequent turns', async () => {
    const turn1 = makeSseResponse([
      'data: {"type":"tool_call","id":"call-1","tool":"list_pods","input":{"namespace":"api-smoke"}}\n\n',
      'data: {"type":"tool_result","id":"call-1","ok":true,"result":{"pods":[]},"error":null}\n\n',
      'data: {"type":"text","delta":"all good"}\n\n',
      'data: {"type":"done"}\n\n',
    ]);
    const turn2 = makeSseResponse([
      'data: {"type":"text","delta":"ack"}\n\n',
      'data: {"type":"done"}\n\n',
    ]);
    let turn = 0;
    const { requests } = captureFetch(() => (turn++ === 0 ? turn1 : turn2));

    renderWithClient(<ChatPanel />);
    await sendMessage('list pods');
    await waitFor(() => expect(screen.getByText('all good')).toBeInTheDocument());

    await sendMessage('thanks');
    await waitFor(() => expect(requests).toHaveLength(2));

    const secondBody = requests[1].body as {
      transcript: { role: string; content: string }[];
      message: string;
    };
    expect(secondBody.message).toBe('thanks');
    // Per the spec: only user + assistant. No tool events.
    for (const entry of secondBody.transcript) {
      expect(['user', 'assistant']).toContain(entry.role);
    }
    // Spec also requires: the prior assistant turn IS included so the runtime
    // sees the conversation history (the "transcript is owned by the FE")
    // invariant.
    const roles = secondBody.transcript.map((entry) => entry.role);
    expect(roles).toEqual(['user', 'assistant']);
    expect(secondBody.transcript[0].content).toBe('list pods');
    expect(secondBody.transcript[1].content).toBe('all good');
  });

  it('streams text deltas across chunks and finalizes on done', async () => {
    const { requests: _r } = captureFetch(() =>
      makeSseResponse([
        'data: {"type":"text","delta":"Hel"}\n\n',
        'data: {"type":"text","delta":"lo"}\n\n',
        'data: {"type":"done"}\n\n',
      ]),
    );
    void _r;

    renderWithClient(<ChatPanel />);
    await sendMessage('hi');

    await waitFor(() => {
      expect(screen.getByText('Hello')).toBeInTheDocument();
    });
  });

  it('renders assistant Markdown while keeping user messages literal', async () => {
    captureFetch(() =>
      makeSseResponse([
        'data: {"type":"text","delta":"**api** deployments:\\n- `api`\\n- worker\\n```\\nkubectl get deploy\\n```"}\n\n',
        'data: {"type":"done"}\n\n',
      ]),
    );

    renderWithClient(<ChatPanel />);
    await sendMessage('show **literal** `markdown`');

    await waitFor(() => {
      expect(screen.getByText('api', { selector: 'strong' })).toBeInTheDocument();
    });

    expect(screen.getByText('show **literal** `markdown`')).toBeInTheDocument();
    expect(screen.getByText('show **literal** `markdown`').querySelector('strong')).toBeNull();
    expect(screen.getByText('api', { selector: 'code' })).toBeInTheDocument();
    expect(screen.getByText('worker').closest('li')).not.toBeNull();
    expect(screen.getByText('kubectl get deploy').tagName).toBe('CODE');
  });

  it('renders assistant headings and pipe tables as formatted Markdown', async () => {
    captureFetch(() =>
      makeSseResponse([
        'data: {"type":"text","delta":"## Cluster State\\n\\n| Name | Replicas | Available | Paused |\\n|------|----------|-----------|--------|\\n| api | 1/1 | 1 | no |"}\n\n',
        'data: {"type":"done"}\n\n',
      ]),
    );

    renderWithClient(<ChatPanel />);
    await sendMessage('show cluster state');

    const heading = await screen.findByRole('heading', { level: 2, name: 'Cluster State' });
    expect(heading).toBeInTheDocument();
    expect(screen.queryByText('## Cluster State')).not.toBeInTheDocument();

    const table = screen.getByRole('table');
    expect(table).toBeInTheDocument();
    expect(screen.getAllByRole('columnheader').map((header) => header.textContent)).toEqual([
      'Name',
      'Replicas',
      'Available',
      'Paused',
    ]);
    expect(screen.getByRole('cell', { name: 'api' })).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: '1/1' })).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: 'no' })).toBeInTheDocument();
    expect(
      screen.queryByText(/\| Name \| Replicas \| Available \| Paused \|/),
    ).not.toBeInTheDocument();
  });

  it('renders assistant GFM beyond headings and tables', async () => {
    captureFetch(() =>
      makeSseResponse([
        'data: {"type":"text","delta":"> quoted status\\n\\n1. first step\\n2. second step\\n\\n[docs](https://example.test/docs)\\n\\n~~stale~~\\n\\n- [x] verified"}\n\n',
        'data: {"type":"done"}\n\n',
      ]),
    );

    renderWithClient(<ChatPanel />);
    await sendMessage('show rich markdown');

    const quotedStatus = await screen.findByText('quoted status');
    expect(quotedStatus.closest('blockquote')).not.toBeNull();

    expect(screen.getByText('first step').closest('ol')).not.toBeNull();
    expect(screen.getByText('second step').closest('ol')).not.toBeNull();

    const docsLink = screen.getByRole('link', { name: 'docs' });
    expect(docsLink).toHaveAttribute('href', 'https://example.test/docs');

    expect(screen.getByText('stale').tagName).toBe('DEL');

    const verifiedCheckbox = screen.getByRole('checkbox');
    expect(verifiedCheckbox).toBeChecked();
    expect(verifiedCheckbox).toBeDisabled();
  });

  it('does not render raw HTML in assistant Markdown as live elements', async () => {
    captureFetch(() =>
      makeSseResponse([
        'data: {"type":"text","delta":"<button>run unsafe action</button>"}\n\n',
        'data: {"type":"done"}\n\n',
      ]),
    );

    renderWithClient(<ChatPanel />);
    await sendMessage('show html');

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'run unsafe action' })).not.toBeInTheDocument();
    });
    expect(screen.getByText('<button>run unsafe action</button>')).toBeInTheDocument();
  });

  it('renders tool_call frames but hides tool_result payloads from the transcript', async () => {
    captureFetch(() =>
      makeSseResponse([
        'data: {"type":"tool_call","id":"c1","tool":"list_pods","input":{"namespace":"api-smoke"}}\n\n',
        'data: {"type":"tool_result","id":"c1","ok":true,"result":{"pods":["api-123"]},"error":null}\n\n',
        'data: {"type":"text","delta":"done thinking"}\n\n',
        'data: {"type":"done"}\n\n',
      ]),
    );

    renderWithClient(<ChatPanel />);
    await sendMessage('list');

    await waitFor(() => {
      expect(screen.getByText('list_pods')).toBeInTheDocument();
    });
    expect(screen.queryByText(/^ok$/)).not.toBeInTheDocument();
    expect(screen.queryByText(/api-123/)).not.toBeInTheDocument();
    expect(screen.getByText('done thinking')).toBeInTheDocument();
  });

  it('ignores SSE heartbeats (`:ping`) and concatenates surrounding text correctly', async () => {
    captureFetch(() =>
      makeSseResponse([
        'data: {"type":"text","delta":"Hel"}\n\n',
        ': keepalive\n\n',
        'data: {"type":"text","delta":"lo"}\n\n',
        'data: {"type":"done"}\n\n',
      ]),
    );

    renderWithClient(<ChatPanel />);
    await sendMessage('hi');

    await waitFor(() => {
      expect(screen.getByText('Hello')).toBeInTheDocument();
    });
  });

  it('disables the input while streaming and re-enables it after done', async () => {
    const queue = makeSseResponseQueue();
    captureFetch(() => queue.response);

    renderWithClient(<ChatPanel />);
    await sendMessage('hold');

    const textarea = await screen.findByPlaceholderText(/Agent is responding…/);
    expect(textarea).toBeDisabled();

    await act(async () => {
      queue.push('data: {"type":"text","delta":"ok"}\n\n');
      queue.push('data: {"type":"done"}\n\n');
      queue.end();
    });

    await waitFor(() => {
      expect(screen.getByPlaceholderText(/Ask the agent…/)).not.toBeDisabled();
    });
  });

  it('exposes a Stop button while streaming and aborts the in-flight fetch', async () => {
    const queue = makeSseResponseQueue();
    const { requests } = captureFetch(() => queue.response);

    renderWithClient(<ChatPanel />);
    await sendMessage('hang');

    const stopButton = await screen.findByRole('button', { name: /Stop/i });
    expect(stopButton).toBeInTheDocument();

    const signal = requests[0].signal;
    expect(signal).not.toBeNull();
    expect(signal!.aborted).toBe(false);

    const user = userEvent.setup();
    await act(async () => {
      await user.click(stopButton);
    });

    expect(signal!.aborted).toBe(true);

    // Cleanup so the test runner doesn't see an unclosed stream.
    await act(async () => {
      queue.abort('test abort');
      // Yield once so the rejection settles inside React's act scope.
      await Promise.resolve();
    });
  });

  it('hides denial tool_result payloads from the transcript', async () => {
    captureFetch(() =>
      makeSseResponse([
        'data: {"type":"tool_call","id":"c1","tool":"scale","input":{"namespace":"app","name":"web","replicas":3}}\n\n',
        'data: {"type":"tool_result","id":"c1","ok":false,"result":{"error":"namespace app is not on the allowed list","decision":{"allow":false,"action":"scale","subject":"app/web","reason":"namespace app is not on the allowed list"}},"error":"namespace app is not on the allowed list"}\n\n',
        'data: {"type":"text","delta":"sorry"}\n\n',
        'data: {"type":"done"}\n\n',
      ]),
    );

    renderWithClient(<ChatPanel />);
    await sendMessage('scale app/web to 3');

    await waitFor(() => {
      expect(screen.getByText('scale')).toBeInTheDocument();
    });
    expect(screen.queryByText(/namespace app is not on the allowed list/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/^denied$/i)).not.toBeInTheDocument();
  });
});
