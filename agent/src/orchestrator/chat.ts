// POST /api/agent/chat handler. Streams Server-Sent Events as defined in
// sseContract.ts. The runtime is stateless: the frontend supplies the
// transcript on every turn and we never persist it.

import type { Request, Response } from 'express';
import { query } from '@anthropic-ai/claude-agent-sdk';

import type { BackendClient } from '../backendClient.js';
import type { Logger } from '../logging.js';
import { buildAgentOptions } from '../agents/agent.js';
import { MCP_SERVER_NAME } from '../agents/tools.js';
import type { ChatRequestBody, ChatTranscriptEntry, SseFrame } from './sseContract.js';

const MCP_TOOL_PREFIX = `mcp__${MCP_SERVER_NAME}__`;
const HEARTBEAT_INTERVAL_MS = 15_000;

export interface ChatHandlerDeps {
  apiKey: string;
  client: BackendClient;
  logger: Logger;
}

export function createChatHandler(
  deps: ChatHandlerDeps,
): (request: Request, response: Response) => Promise<void> {
  return async function handleChat(request, response) {
    const parsed = parseBody(request.body);
    if (!parsed.ok) {
      response.status(400).json({ error: parsed.error });
      return;
    }
    const { transcript, message } = parsed.value;
    deps.logger.info('agent.chat_start', {
      messagePreview: message.slice(0, 60),
      transcriptLen: transcript.length,
    });

    response.setHeader('Content-Type', 'text/event-stream');
    response.setHeader('Cache-Control', 'no-cache, no-transform');
    response.setHeader('Connection', 'keep-alive');
    response.setHeader('X-Accel-Buffering', 'no');
    response.flushHeaders();

    const heartbeat = setInterval(() => {
      try {
        response.write(': ping\n\n');
      } catch {
        /* connection already closed */
      }
    }, HEARTBEAT_INTERVAL_MS);

    function send(frame: SseFrame): void {
      response.write(`data: ${JSON.stringify(frame)}\n\n`);
    }

    const abortController = new AbortController();
    let agentSettled = false;
    // Listen on the response, not the request: the response's "close" only
    // fires when the underlying socket is gone, so it is the right signal
    // for "client disconnected mid-stream." `request.on("close")` fires as
    // soon as the request body finishes, which on a body-having POST is
    // immediately after json parsing — and would abort every run.
    response.on('close', () => {
      if (!agentSettled) {
        abortController.abort();
      }
    });

    try {
      await runAgentStream({
        apiKey: deps.apiKey,
        client: deps.client,
        logger: deps.logger,
        transcript,
        message,
        abortController,
        send,
      });
      agentSettled = true;
      send({ type: 'done' });
    } catch (caught) {
      agentSettled = true;
      const reason = caught instanceof Error ? caught.message : String(caught);
      const stack = caught instanceof Error ? caught.stack : undefined;
      deps.logger.error('agent.stream_error', { reason, stack });
      try {
        send({ type: 'error', message: reason });
      } catch {
        /* connection already closed */
      }
    } finally {
      clearInterval(heartbeat);
      if (!response.writableEnded) {
        response.end();
      }
    }
  };
}

interface RunArgs {
  apiKey: string;
  client: BackendClient;
  logger: Logger;
  transcript: ChatTranscriptEntry[];
  message: string;
  abortController: AbortController;
  send: (frame: SseFrame) => void;
}

async function runAgentStream(args: RunArgs): Promise<void> {
  const { apiKey, client, logger, transcript, message, abortController, send } = args;

  const options = buildAgentOptions({ apiKey, client, abortController });

  const composedPrompt = composePrompt(transcript, message);
  const stream = query({ prompt: composedPrompt, options });

  const emittedText = new Set<string>();

  for await (const event of stream) {
    logger.debug('agent.sdk_event', {
      eventType: event.type,
      subtype: (event as { subtype?: string }).subtype,
    });
    if (event.type === 'assistant') {
      const blocks = event.message?.content ?? [];
      for (const block of blocks) {
        if (block.type === 'text') {
          // Each assistant message arrives as a complete block; emit any text
          // we have not yet sent. (We are not enabling partial streaming, so
          // this fires once per assistant turn per text block.)
          const dedupKey = `${event.uuid}:${block.text}`;
          if (!emittedText.has(dedupKey)) {
            emittedText.add(dedupKey);
            send({ type: 'text', delta: block.text });
          }
        } else if (block.type === 'tool_use') {
          const friendly = stripMcpPrefix(block.name);
          send({
            type: 'tool_call',
            id: block.id,
            tool: friendly,
            input: (block.input ?? {}) as Record<string, unknown>,
          });
        }
      }
    } else if (event.type === 'user') {
      // The SDK emits synthetic user messages carrying tool_result blocks
      // after each tool runs. Surface them as tool_result frames.
      const messageContent = event.message?.content;
      if (Array.isArray(messageContent)) {
        for (const block of messageContent) {
          if (
            typeof block === 'object' &&
            block !== null &&
            (block as { type?: string }).type === 'tool_result'
          ) {
            const toolResultBlock = block as {
              tool_use_id: string;
              content?: unknown;
              is_error?: boolean;
            };
            const { ok, error, result } = interpretToolResult(toolResultBlock);
            send({
              type: 'tool_result',
              id: toolResultBlock.tool_use_id,
              ok,
              result,
              error,
            });
          }
        }
      }
    } else if (event.type === 'result') {
      if (event.subtype !== 'success') {
        const errors = event.errors?.join('; ') || event.subtype;
        throw new Error(`agent run ended with ${event.subtype}: ${errors}`);
      }
      // Success result terminates the run; nothing more to translate.
    } else if (event.type === 'system' && event.subtype === 'permission_denied') {
      logger.warn('agent.permission_denied', {
        tool: event.tool_name,
        reason: event.decision_reason ?? null,
      });
    }
  }
}

function interpretToolResult(block: { content?: unknown; is_error?: boolean }): {
  ok: boolean;
  error: string | null;
  result: unknown;
} {
  const rawText = stringifyToolContent(block.content);
  let parsed: unknown = rawText;
  if (typeof rawText === 'string' && rawText.length > 0) {
    try {
      parsed = JSON.parse(rawText);
    } catch {
      parsed = rawText;
    }
  }

  if (block.is_error) {
    const errorMessage =
      typeof parsed === 'string' ? parsed : (extractErrorString(parsed) ?? 'tool returned error');
    return { ok: false, error: errorMessage, result: parsed };
  }

  // Tool returned 2xx but the body indicates a denial or error from the
  // backend (the BackendClient marks these with denied/error fields).
  if (parsed && typeof parsed === 'object') {
    const payload = parsed as Record<string, unknown>;
    if (payload.denied === true) {
      const reason = typeof payload.reason === 'string' ? payload.reason : 'denied by backend';
      return { ok: false, error: reason, result: parsed };
    }
    if (payload.error === true) {
      const message = typeof payload.message === 'string' ? payload.message : 'backend error';
      return { ok: false, error: message, result: parsed };
    }
  }
  return { ok: true, error: null, result: parsed };
}

function stringifyToolContent(content: unknown): string {
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    return content
      .map((block) => {
        if (typeof block === 'string') return block;
        if (block && typeof block === 'object' && (block as { type?: string }).type === 'text') {
          return (block as { text?: string }).text ?? '';
        }
        return JSON.stringify(block);
      })
      .join('');
  }
  if (content === undefined || content === null) return '';
  return JSON.stringify(content);
}

function extractErrorString(value: unknown): string | undefined {
  if (value && typeof value === 'object') {
    const candidate =
      (value as Record<string, unknown>).error ??
      (value as Record<string, unknown>).message ??
      (value as Record<string, unknown>).reason;
    if (typeof candidate === 'string') return candidate;
  }
  return undefined;
}

function stripMcpPrefix(toolName: string): string {
  if (toolName.startsWith(MCP_TOOL_PREFIX)) {
    return toolName.slice(MCP_TOOL_PREFIX.length);
  }
  return toolName;
}

interface ParsedBody {
  ok: true;
  value: ChatRequestBody;
}
interface ParseError {
  ok: false;
  error: string;
}

function parseBody(body: unknown): ParsedBody | ParseError {
  if (!body || typeof body !== 'object') {
    return { ok: false, error: 'request body must be a JSON object' };
  }
  const candidate = body as Partial<ChatRequestBody>;
  if (typeof candidate.message !== 'string' || candidate.message.trim().length === 0) {
    return { ok: false, error: 'message is required and must be a non-empty string' };
  }
  if (!Array.isArray(candidate.transcript)) {
    return { ok: false, error: 'transcript is required and must be an array' };
  }
  for (const entry of candidate.transcript) {
    if (!entry || typeof entry !== 'object') {
      return { ok: false, error: 'transcript entries must be objects' };
    }
    const role = (entry as ChatTranscriptEntry).role;
    if (role !== 'user' && role !== 'assistant') {
      return { ok: false, error: 'transcript entry role must be "user" or "assistant"' };
    }
    if (typeof (entry as ChatTranscriptEntry).content !== 'string') {
      return { ok: false, error: 'transcript entry content must be a string' };
    }
  }
  return {
    ok: true,
    value: {
      transcript: candidate.transcript as ChatTranscriptEntry[],
      message: candidate.message,
    },
  };
}

// Build the prompt string the SDK runs against. We are stateless — the
// frontend supplies the full transcript on every turn — so we serialize the
// prior turns inline and let the live user message be the new request. The
// system prompt already carries behavior, so the transcript here is purely
// conversation context.
function composePrompt(transcript: ChatTranscriptEntry[], message: string): string {
  if (transcript.length === 0) {
    return message;
  }
  const lines: string[] = ['Prior conversation (most recent last):', '<transcript>'];
  for (const entry of transcript) {
    const speaker = entry.role === 'user' ? 'Operator' : 'Agent';
    lines.push(`${speaker}: ${entry.content}`);
  }
  lines.push('</transcript>');
  lines.push('');
  lines.push(`New operator message: ${message}`);
  return lines.join('\n');
}
