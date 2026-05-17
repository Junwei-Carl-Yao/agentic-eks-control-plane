// CLI entrypoint for the eval harness. Spins up the mock backend, runs each
// scenario through the real Anthropic API using the same agent code path the
// production runtime uses, then evaluates the recorded behavior against the
// scenario expectations.

import { resolve } from 'node:path';
import { query } from '@anthropic-ai/claude-agent-sdk';

import { BackendClient } from '../../src/backendClient.js';
import { loadConfig } from '../../src/config.js';
import { buildAgentOptions } from '../../src/agents/agent.js';
import { MCP_SERVER_NAME } from '../../src/agents/tools.js';
import { MUTATION_TOOLS, SCENARIOS, type Scenario, type ToolExpectation } from './dataset.js';
import { startMockBackend, type RecordedCall } from './mockBackend.js';
import { buildReport, printReport, writeReport, type ScenarioOutcome } from './report.js';

const MCP_PREFIX = `mcp__${MCP_SERVER_NAME}__`;

interface CliArgs {
  filter?: string;
}

function parseArgs(argv: string[]): CliArgs {
  const args: CliArgs = {};
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (token === '--filter' && index + 1 < argv.length) {
      args.filter = argv[index + 1];
      index += 1;
    } else if (token?.startsWith('--filter=')) {
      args.filter = token.slice('--filter='.length);
    }
  }
  return args;
}

async function main(): Promise<void> {
  const cli = parseArgs(process.argv.slice(2));
  const scenarios = cli.filter
    ? SCENARIOS.filter((scenario) => scenario.name === cli.filter)
    : SCENARIOS;
  if (scenarios.length === 0) {
    process.stderr.write(`No scenarios matched filter ${cli.filter}\n`);
    process.exit(1);
  }

  const config = loadConfig();
  const mock = await startMockBackend();
  const client = new BackendClient(mock.url);
  process.stdout.write(`Mock backend listening on ${mock.url}\n`);
  process.stdout.write(
    `Running ${scenarios.length} scenario${scenarios.length === 1 ? '' : 's'}\n\n`,
  );

  const outcomes: ScenarioOutcome[] = [];
  try {
    for (const scenario of scenarios) {
      process.stdout.write(`> ${scenario.name}\n`);
      mock.reset();
      const outcome = await runScenario(scenario, config.anthropicApiKey, client, mock.calls);
      outcomes.push(outcome);
    }
  } finally {
    await mock.stop();
  }

  const report = buildReport(outcomes);
  printReport(report);
  const reportPath = resolve(process.cwd(), 'test/evals/report.json');
  writeReport(reportPath, report);
  process.stdout.write(`Report written to ${reportPath}\n`);

  process.exit(report.failed === 0 ? 0 : 1);
}

interface ToolCallRecord {
  id: string;
  tool: string;
  input: Record<string, unknown>;
  ok: boolean;
  error: string | null;
  result: unknown;
}

async function runScenario(
  scenario: Scenario,
  apiKey: string,
  client: BackendClient,
  recordedCalls: RecordedCall[],
): Promise<ScenarioOutcome> {
  const startTime = Date.now();
  const abortController = new AbortController();
  const options = buildAgentOptions({ apiKey, client, abortController });

  const promptText = composePrompt(scenario);
  const stream = query({ prompt: promptText, options });

  const toolCalls: ToolCallRecord[] = [];
  const callsById = new Map<string, ToolCallRecord>();
  const textChunks: string[] = [];
  let runError: string | null = null;

  try {
    for await (const event of stream) {
      if (event.type === 'assistant') {
        for (const block of event.message?.content ?? []) {
          if (block.type === 'text') {
            textChunks.push(block.text);
          } else if (block.type === 'tool_use') {
            const friendly = stripPrefix(block.name);
            const record: ToolCallRecord = {
              id: block.id,
              tool: friendly,
              input: (block.input ?? {}) as Record<string, unknown>,
              ok: true,
              error: null,
              result: null,
            };
            toolCalls.push(record);
            callsById.set(block.id, record);
          }
        }
      } else if (event.type === 'user') {
        const messageContent = event.message?.content;
        if (Array.isArray(messageContent)) {
          for (const block of messageContent) {
            if (
              block &&
              typeof block === 'object' &&
              (block as { type?: string }).type === 'tool_result'
            ) {
              const toolResult = block as {
                tool_use_id: string;
                content?: unknown;
                is_error?: boolean;
              };
              const record = callsById.get(toolResult.tool_use_id);
              if (record) {
                const interpreted = interpretToolResult(toolResult);
                record.ok = interpreted.ok;
                record.error = interpreted.error;
                record.result = interpreted.result;
              }
            }
          }
        }
      } else if (event.type === 'result') {
        if (event.subtype !== 'success') {
          runError = `agent run ended with ${event.subtype}: ${(event.errors ?? []).join('; ')}`;
        }
      }
    }
  } catch (caught) {
    runError = caught instanceof Error ? caught.message : String(caught);
  }

  const failures = evaluateScenario(scenario, toolCalls, recordedCalls, runError);
  const passed = failures.length === 0;
  return {
    name: scenario.name,
    category: scenario.category,
    passed,
    failures,
    toolCalls: toolCalls.map((call) => ({
      tool: call.tool,
      input: call.input,
      ok: call.ok,
      error: call.error,
    })),
    textSummary: textChunks.join('\n'),
    durationMs: Date.now() - startTime,
  };
}

function composePrompt(scenario: Scenario): string {
  if (!scenario.transcript || scenario.transcript.length === 0) {
    return scenario.message;
  }
  const lines: string[] = ['Prior conversation (most recent last):', '<transcript>'];
  for (const entry of scenario.transcript) {
    const speaker = entry.role === 'user' ? 'Operator' : 'Agent';
    lines.push(`${speaker}: ${entry.content}`);
  }
  lines.push('</transcript>');
  lines.push('');
  lines.push(`New operator message: ${scenario.message}`);
  return lines.join('\n');
}

function evaluateScenario(
  scenario: Scenario,
  toolCalls: ToolCallRecord[],
  recordedCalls: RecordedCall[],
  runError: string | null,
): string[] {
  const failures: string[] = [];
  if (runError) failures.push(runError);

  // forbidTools: any call to one of these is an immediate fail.
  if (scenario.forbidTools) {
    for (const forbidden of scenario.forbidTools) {
      const offendingCall = toolCalls.find((call) => call.tool === forbidden);
      if (offendingCall) {
        failures.push(
          `forbidden tool ${forbidden} was called with ${JSON.stringify(offendingCall.input)}`,
        );
      }
    }
  }

  // maxMutationCalls
  if (typeof scenario.maxMutationCalls === 'number') {
    const mutationCount = toolCalls.filter((call) => MUTATION_TOOLS.has(call.tool)).length;
    if (mutationCount > scenario.maxMutationCalls) {
      failures.push(
        `expected at most ${scenario.maxMutationCalls} mutation tool calls, observed ${mutationCount}`,
      );
    }
  }

  // expectTools must each match (in order) a tool call we recorded.
  let cursor = 0;
  for (const expectation of scenario.expectTools) {
    let matchIndex = -1;
    for (let index = cursor; index < toolCalls.length; index += 1) {
      if (matchesExpectation(toolCalls[index]!, expectation)) {
        matchIndex = index;
        break;
      }
    }
    if (matchIndex === -1) {
      failures.push(
        `expected tool call to ${expectation.tool} (input contains ${JSON.stringify(expectation.inputContains ?? {})}, denied=${expectation.denied ?? false}) was not found`,
      );
      continue;
    }
    cursor = matchIndex + 1;
  }

  // Adversarial scenarios: also verify the mock backend recorded zero
  // mutation requests when maxMutationCalls is 0.
  if (scenario.maxMutationCalls === 0) {
    const mutationOnBackend = recordedCalls.filter((call) =>
      call.path.startsWith('/api/operations/'),
    );
    if (mutationOnBackend.length > 0) {
      failures.push(
        `expected zero mutation requests at the backend, observed ${mutationOnBackend.length}`,
      );
    }
  }

  return failures;
}

function matchesExpectation(call: ToolCallRecord, expectation: ToolExpectation): boolean {
  if (call.tool !== expectation.tool) return false;
  if (expectation.denied !== undefined) {
    if (expectation.denied && call.ok) return false;
    if (!expectation.denied && !call.ok) return false;
  }
  if (expectation.inputContains) {
    for (const [key, expectedValue] of Object.entries(expectation.inputContains)) {
      if (!Object.is(call.input[key], expectedValue)) return false;
    }
  }
  return true;
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
    const message =
      typeof parsed === 'string'
        ? parsed
        : (extractField(parsed, 'error') ??
          extractField(parsed, 'message') ??
          'tool returned error');
    return { ok: false, error: message, result: parsed };
  }
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

function extractField(value: unknown, key: string): string | undefined {
  if (value && typeof value === 'object' && key in value) {
    const candidate = (value as Record<string, unknown>)[key];
    if (typeof candidate === 'string') return candidate;
  }
  return undefined;
}

function stripPrefix(toolName: string): string {
  if (toolName.startsWith(MCP_PREFIX)) {
    return toolName.slice(MCP_PREFIX.length);
  }
  return toolName;
}

main().catch((caught) => {
  const reason = caught instanceof Error ? caught.message : String(caught);
  process.stderr.write(`Eval harness failed: ${reason}\n`);
  process.exit(1);
});
