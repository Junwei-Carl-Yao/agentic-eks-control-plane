// Builds the Options object handed to the Claude Agent SDK's query() call.
// Stateless: nothing here holds session state — the orchestrator constructs
// a fresh options bundle per request from the supplied transcript.

import type { Options } from '@anthropic-ai/claude-agent-sdk';

import type { BackendClient } from '../backendClient.js';
import { AGENT_SYSTEM } from './prompts.js';
import { ALLOWED_TOOL_NAMES, MCP_SERVER_NAME, buildKubernetesMcpServer } from './tools.js';

const MODEL = 'claude-opus-4-7';

export interface AgentOptionsInput {
  apiKey: string;
  client: BackendClient;
  abortController?: AbortController;
}

export function buildAgentOptions(input: AgentOptionsInput): Options {
  const mcpServer = buildKubernetesMcpServer(input.client);
  return {
    model: MODEL,
    systemPrompt: AGENT_SYSTEM,
    // Disable every built-in Claude Code tool — only our backend-wrapped MCP
    // tools may run. The frontend never asks the agent to read files or run
    // shell commands.
    tools: [],
    mcpServers: {
      [MCP_SERVER_NAME]: mcpServer,
    },
    // Auto-approve every backend-wrapped tool. The backend enforcer is the
    // chokepoint; per-call permission prompts here would just be theater.
    allowedTools: ALLOWED_TOOL_NAMES,
    permissionMode: 'bypassPermissions',
    allowDangerouslySkipPermissions: true,
    // Keep the runtime hermetic: no on-disk settings, no project plugins, no
    // CLAUDE.md cascade. The agent runs only with what we hand it.
    settingSources: [],
    persistSession: false,
    includePartialMessages: false,
    abortController: input.abortController,
    env: {
      ...process.env,
      ANTHROPIC_API_KEY: input.apiKey,
    },
  };
}

export { MODEL as AGENT_MODEL };
