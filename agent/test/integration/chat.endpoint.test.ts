// Sections C + D: orchestrator HTTP behavior, statelessness, and SSE framing.
//
// We boot the chat handler in-process using express, replace the Claude
// Agent SDK with a vi.mock so no network calls happen, and assert against
// the documented wire contract:
//   - POST /api/agent/chat reads {transcript, message}
//   - 400 JSON for malformed bodies (NOT SSE)
//   - text/event-stream + no-cache headers
//   - tool_call frame paired with tool_result frame, by id, in order
//   - exactly one {type: "done"} frame; stream closes after
//   - errors emit {type: "error", message: ...}; key never leaks
//   - statelessness: independent requests do not share transcript
//   - the mocked SDK is invoked with the supplied transcript content + new
//     message
//
// We carefully construct a fake SDK module so each call to query() consumes
// a fresh script. The script is set per test before invoking the handler.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import express, { type Express } from 'express';
import type { AddressInfo } from 'node:net';
import type { Server } from 'node:http';

// ---- SDK mock setup --------------------------------------------------------
// The SDK exports `query({prompt, options})` returning an async iterable of
// SDK messages. We replay a configurable script of events per call.

interface ScriptedTextBlock {
  type: 'text';
  text: string;
}
interface ScriptedToolUseBlock {
  type: 'tool_use';
  id: string;
  name: string;
  input: Record<string, unknown>;
}
type ScriptedAssistantBlock = ScriptedTextBlock | ScriptedToolUseBlock;

interface ScriptedToolResult {
  tool_use_id: string;
  content: string;
  is_error?: boolean;
}

type ScriptedEvent =
  | { type: 'assistant'; uuid: string; message: { content: ScriptedAssistantBlock[] } }
  | { type: 'user'; message: { content: ({ type: 'tool_result' } & ScriptedToolResult)[] } }
  | { type: 'result'; subtype: 'success' }
  | { type: 'result'; subtype: 'error'; errors?: string[] };

interface QueryInvocation {
  prompt: string;
  options: { model?: string; systemPrompt?: string; env?: Record<string, string | undefined> };
}

const queryInvocations: QueryInvocation[] = [];
let scriptQueue: ScriptedEvent[][] = [];
let nextThrowError: Error | null = null;

vi.mock('@anthropic-ai/claude-agent-sdk', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@anthropic-ai/claude-agent-sdk')>();
  return {
    ...actual,
    query: (args: QueryInvocation) => {
      queryInvocations.push(args);
      if (nextThrowError) {
        const error = nextThrowError;
        nextThrowError = null;
        // The handler awaits the async iterator; throw inside the iterator so
        // it surfaces as a thrown error mid-iteration.
        return (async function* () {
          throw error;
          // eslint-disable-next-line no-unreachable
          yield undefined as never;
        })();
      }
      const eventsForCall = scriptQueue.shift() ?? [
        { type: 'result', subtype: 'success' } as ScriptedEvent,
      ];
      return (async function* () {
        for (const event of eventsForCall) {
          yield event;
        }
      })();
    },
  };
});

// Imports must come AFTER the vi.mock above so the mock is hoisted in place.
const { createChatHandler } = await import('../../src/orchestrator/chat.js');
const { BackendClient } = await import('../../src/backendClient.js');

// ---- Express test server ---------------------------------------------------

const FAKE_API_KEY = 'sk-LEAK-CANARY-DO-NOT-LOG-THIS-VALUE-12345';

interface ServerHandle {
  app: Express;
  server: Server;
  port: number;
  loggerCalls: { level: string; message: string; fields?: Record<string, unknown> }[];
}

function startServer(): Promise<ServerHandle> {
  const loggerCalls: ServerHandle['loggerCalls'] = [];
  const logger = {
    debug: (message: string, fields?: Record<string, unknown>) =>
      loggerCalls.push({ level: 'debug', message, fields }),
    info: (message: string, fields?: Record<string, unknown>) =>
      loggerCalls.push({ level: 'info', message, fields }),
    warn: (message: string, fields?: Record<string, unknown>) =>
      loggerCalls.push({ level: 'warn', message, fields }),
    error: (message: string, fields?: Record<string, unknown>) =>
      loggerCalls.push({ level: 'error', message, fields }),
  };
  const handler = createChatHandler({
    apiKey: FAKE_API_KEY,
    client: new BackendClient('http://backend.test'),
    logger,
  });
  const app = express();
  app.use(express.json({ limit: '1mb' }));
  app.post('/api/agent/chat', (request, response) => {
    handler(request, response).catch(() => {
      if (!response.headersSent) response.status(500).end();
      else if (!response.writableEnded) response.end();
    });
  });
  return new Promise((resolveServer) => {
    const server = app.listen(0, () => {
      const address = server.address() as AddressInfo;
      resolveServer({ app, server, port: address.port, loggerCalls });
    });
  });
}

function stopServer(handle: ServerHandle): Promise<void> {
  return new Promise((resolveServer) => {
    handle.server.close(() => resolveServer());
  });
}

// ---- SSE parsing helpers ---------------------------------------------------

interface SseFrame {
  type: string;
  [key: string]: unknown;
}

async function postChat(
  port: number,
  body: unknown,
  contentType = 'application/json',
): Promise<Response> {
  return fetch(`http://127.0.0.1:${port}/api/agent/chat`, {
    method: 'POST',
    headers: { 'Content-Type': contentType },
    body: typeof body === 'string' ? body : JSON.stringify(body),
  });
}

async function readSseFrames(response: Response): Promise<{ frames: SseFrame[]; raw: string }> {
  const text = await response.text();
  const frames: SseFrame[] = [];
  for (const line of text.split(/\n\n/)) {
    const trimmed = line.trim();
    if (trimmed.length === 0) continue;
    if (trimmed.startsWith(':')) continue; // SSE comment / heartbeat
    if (!trimmed.startsWith('data:')) continue;
    const jsonText = trimmed.slice('data:'.length).trim();
    try {
      frames.push(JSON.parse(jsonText) as SseFrame);
    } catch {
      // intentionally ignore malformed frames in the test parser
    }
  }
  return { frames, raw: text };
}

// ---- Tests -----------------------------------------------------------------

describe('POST /api/agent/chat', () => {
  let handle: ServerHandle;

  beforeEach(async () => {
    queryInvocations.length = 0;
    scriptQueue = [];
    nextThrowError = null;
    handle = await startServer();
  });

  afterEach(async () => {
    await stopServer(handle);
  });

  // C.10: malformed bodies must reject with 400 JSON, not SSE.
  describe('body validation', () => {
    it('rejects an empty body with 400 JSON', async () => {
      const response = await postChat(handle.port, '');
      expect(response.status).toBe(400);
      expect(response.headers.get('content-type') ?? '').toContain('application/json');
    });

    it('rejects a missing message with 400 JSON', async () => {
      const response = await postChat(handle.port, { transcript: [] });
      expect(response.status).toBe(400);
      const ct = response.headers.get('content-type') ?? '';
      expect(ct).toContain('application/json');
      expect(ct).not.toContain('text/event-stream');
      const body = (await response.json()) as { error: string };
      expect(typeof body.error).toBe('string');
    });

    it('rejects a wrong-shape transcript with 400 JSON', async () => {
      const response = await postChat(handle.port, { transcript: 'nope', message: 'hi' });
      expect(response.status).toBe(400);
      expect(response.headers.get('content-type') ?? '').toContain('application/json');
    });

    it('rejects transcript entries with bad role/content with 400 JSON', async () => {
      const response = await postChat(handle.port, {
        transcript: [{ role: 'system', content: 'x' }],
        message: 'hi',
      });
      expect(response.status).toBe(400);
    });

    it('rejects an empty message with 400 JSON', async () => {
      const response = await postChat(handle.port, { transcript: [], message: '   ' });
      expect(response.status).toBe(400);
    });
  });

  // C.11: response headers carry the SSE content-type and no-cache.
  it('sets Content-Type: text/event-stream and Cache-Control: no-cache on success', async () => {
    scriptQueue = [[{ type: 'result', subtype: 'success' }]];
    const response = await postChat(handle.port, { transcript: [], message: 'hello' });
    expect(response.status).toBe(200);
    expect(response.headers.get('content-type') ?? '').toContain('text/event-stream');
    expect(response.headers.get('cache-control') ?? '').toContain('no-cache');
    await response.text(); // drain
  });

  // C.13 + C.14: tool_call paired with tool_result by id, single done frame.
  it('emits tool_call before tool_result, paired by id, then done', async () => {
    scriptQueue = [
      [
        {
          type: 'assistant',
          uuid: 'msg-1',
          message: {
            content: [
              { type: 'text', text: 'Looking up deployments.' },
              {
                type: 'tool_use',
                id: 'tool-call-1',
                name: 'mcp__kubernetes__list_deployments',
                input: { namespace: 'api-smoke' },
              },
            ],
          },
        },
        {
          type: 'user',
          message: {
            content: [
              {
                type: 'tool_result',
                tool_use_id: 'tool-call-1',
                content: JSON.stringify([{ name: 'web', replicas: 2 }]),
              },
            ],
          },
        },
        {
          type: 'assistant',
          uuid: 'msg-2',
          message: {
            content: [{ type: 'text', text: 'Found 1 deployment: web (2 replicas).' }],
          },
        },
        { type: 'result', subtype: 'success' },
      ],
    ];
    const response = await postChat(handle.port, { transcript: [], message: "what's running" });
    const { frames } = await readSseFrames(response);

    const toolCallIndex = frames.findIndex((frame) => frame.type === 'tool_call');
    const toolResultIndex = frames.findIndex((frame) => frame.type === 'tool_result');
    const doneIndex = frames.findIndex((frame) => frame.type === 'done');

    expect(toolCallIndex).toBeGreaterThanOrEqual(0);
    expect(toolResultIndex).toBeGreaterThan(toolCallIndex);
    expect(doneIndex).toBe(frames.length - 1);
    expect(frames.filter((frame) => frame.type === 'done')).toHaveLength(1);

    const toolCall = frames[toolCallIndex] as unknown as {
      id: string;
      tool: string;
      input: Record<string, unknown>;
    };
    const toolResult = frames[toolResultIndex] as unknown as { id: string; ok: boolean };
    expect(toolCall.id).toBe(toolResult.id);
    expect(toolCall.tool).toBe('list_deployments'); // mcp prefix stripped
    expect(toolCall.input).toEqual({ namespace: 'api-smoke' });
    expect(toolResult.ok).toBe(true);
  });

  // C.15: errors emit {type: "error"} and the API key never appears.
  it("emits {type:'error'} on a thrown agent error and never leaks the api key", async () => {
    nextThrowError = new Error('model is overloaded');
    scriptQueue = [];
    const response = await postChat(handle.port, { transcript: [], message: 'hello' });
    const { frames, raw } = await readSseFrames(response);
    const errorFrame = frames.find((frame) => frame.type === 'error');
    expect(errorFrame, `frames: ${JSON.stringify(frames)}`).toBeDefined();
    expect((errorFrame as unknown as { message: string }).message).toContain('model is overloaded');
    // The fake key must not appear ANYWHERE in the SSE stream.
    expect(raw.includes(FAKE_API_KEY)).toBe(false);
    // It also must not appear in any logger call.
    const allLog = JSON.stringify(handle.loggerCalls);
    expect(allLog.includes(FAKE_API_KEY)).toBe(false);
  });

  // D.16 + C.12: transcript is forwarded; runtime is stateless.
  it('forwards the transcript content into the SDK prompt and is stateless across requests', async () => {
    scriptQueue = [
      [{ type: 'result', subtype: 'success' }],
      [{ type: 'result', subtype: 'success' }],
    ];

    const firstResponse = await postChat(handle.port, {
      transcript: [
        { role: 'user', content: 'scale web to 2' },
        { role: 'assistant', content: 'scaled to 2' },
      ],
      message: 'now scale to 3',
    });
    await firstResponse.text();

    const secondResponse = await postChat(handle.port, {
      transcript: [{ role: 'user', content: 'what nodes are there' }],
      message: 'list deployments in api-smoke',
    });
    await secondResponse.text();

    expect(queryInvocations).toHaveLength(2);
    const firstInvocation = queryInvocations[0]!;
    const secondInvocation = queryInvocations[1]!;

    // First request: the prompt must contain BOTH the prior transcript and
    // the new message.
    expect(firstInvocation.prompt).toContain('scale web to 2');
    expect(firstInvocation.prompt).toContain('scaled to 2');
    expect(firstInvocation.prompt).toContain('now scale to 3');

    // Second request: must not contain anything from the first transcript.
    // Statelessness means each request starts from the body alone.
    expect(secondInvocation.prompt).not.toContain('scale web to 2');
    expect(secondInvocation.prompt).not.toContain('scaled to 2');
    expect(secondInvocation.prompt).not.toContain('now scale to 3');
    expect(secondInvocation.prompt).toContain('what nodes are there');
    expect(secondInvocation.prompt).toContain('list deployments in api-smoke');
  });

  it('treats an empty transcript as just the message (no leakage between calls)', async () => {
    scriptQueue = [[{ type: 'result', subtype: 'success' }]];
    const response = await postChat(handle.port, { transcript: [], message: 'hi alone' });
    await response.text();
    expect(queryInvocations).toHaveLength(1);
    expect(queryInvocations[0]!.prompt).toBe('hi alone');
  });

  it('uses claude-opus-4-7 as the model for every request', async () => {
    scriptQueue = [[{ type: 'result', subtype: 'success' }]];
    const response = await postChat(handle.port, { transcript: [], message: 'hi' });
    await response.text();
    expect(queryInvocations[0]!.options.model).toBe('claude-opus-4-7');
  });

  // C.14: stream ends after the done frame — no further bytes.
  it('closes the stream after the done frame on a successful run', async () => {
    scriptQueue = [
      [
        {
          type: 'assistant',
          uuid: 'msg-1',
          message: { content: [{ type: 'text', text: 'ok' }] },
        },
        { type: 'result', subtype: 'success' },
      ],
    ];
    const response = await postChat(handle.port, { transcript: [], message: 'hi' });
    const { frames } = await readSseFrames(response);
    const doneIndices = frames
      .map((frame, index) => ({ index, type: frame.type }))
      .filter((entry) => entry.type === 'done');
    expect(doneIndices).toHaveLength(1);
    expect(doneIndices[0]!.index).toBe(frames.length - 1);
  });
});

// D.17: chat.ts contains no module-scope mutable state for transcripts.
describe('orchestrator/chat.ts has no module-scope transcript state', () => {
  it('the source file declares no module-level Map/object holding sessions', async () => {
    const fs = await import('node:fs');
    const url = await import('node:url');
    const path = await import('node:path');
    const here = url.fileURLToPath(new URL('.', import.meta.url));
    const chatPath = path.join(here, '..', '..', 'src', 'orchestrator', 'chat.ts');
    const source = fs.readFileSync(chatPath, 'utf-8');
    // No module-scope `let sessions =` / `const sessions =` etc.
    expect(source).not.toMatch(/^(?:let|var|const)\s+\w*sessions?\b/m);
    expect(source).not.toMatch(/^(?:let|var|const)\s+\w*transcripts?\b/m);
    expect(source).not.toMatch(/^(?:let|var|const)\s+\w+\s*=\s*new\s+Map\b/m);
  });
});
