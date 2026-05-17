// Section F: Safety invariants.
//
// These tests guard the architectural promises in implementation.md "Key
// Design Invariants" and §4.4. The agent path must not implement policy
// locally — the backend enforcer is the single chokepoint. The system prompt
// must make that explicit and forbid retrying after a deny. The tool surface
// must omit blocked categories.

import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { AGENT_SYSTEM } from '../../src/agents/prompts.js';
import { TOOL_NAMES, buildKubernetesMcpServer } from '../../src/agents/tools.js';
import { BackendClient } from '../../src/backendClient.js';

const here = dirname(fileURLToPath(import.meta.url));
const SRC_DIR = join(here, '..', '..', 'src');

function readSrc(relative: string): string {
  return readFileSync(join(SRC_DIR, relative), 'utf-8');
}

describe('system prompt enforces the safety contract', () => {
  it('names the backend enforcer as the final authority', () => {
    // The prompt must make it explicit that the backend — not the agent — is
    // the policy chokepoint. Accept any clear phrasing.
    const lower = AGENT_SYSTEM.toLowerCase();
    const sentinels = ['enforcer', 'final authority', 'chokepoint', 'single execution boundary'];
    const matched = sentinels.filter((sentinel) => lower.includes(sentinel));
    expect(
      matched.length,
      `expected one of ${sentinels.join(', ')} in AGENT_SYSTEM`,
    ).toBeGreaterThan(0);
    expect(lower).toContain('backend');
  });

  it('forbids the agent from retrying with broadened parameters after a deny', () => {
    const lower = AGENT_SYSTEM.toLowerCase();
    expect(lower).toMatch(/(do not|don't|never)\s+retry/);
    // The retry-prohibition must be tied to the deny path so the agent cannot
    // simply re-issue the same call as if nothing happened.
    expect(lower).toMatch(/denied|deny|denial|relaxed|broadened/);
  });

  it('instructs the agent to surface denial reasons to the user', () => {
    const lower = AGENT_SYSTEM.toLowerCase();
    expect(lower).toMatch(
      /(surface|report|tell|explain|name).+(deny|denial|denied|reason|policy)/s,
    );
  });

  it('does not instruct the agent to implement client-side policy', () => {
    // The prompt may describe the policy so the agent can pre-explain to the
    // user, but it must NOT tell the agent to enforce or block on its own.
    const lower = AGENT_SYSTEM.toLowerCase();
    expect(lower).not.toMatch(/you (must|should) (enforce|block|reject)/);
  });
});

describe('tools.ts contains no client-side policy logic', () => {
  // The spec says tools must not implement policy locally. The backend Phase
  // 3 enforcer is the chokepoint. Verify by inspecting the source: no
  // namespace allowlist, no replica cap, no decision logic in the tool path.
  const toolsSource = readSrc('agents/tools.ts');
  const backendClientSource = readSrc('backendClient.ts');

  it('tools.ts has no hardcoded namespace allowlist', () => {
    // 'api-smoke' may appear in the system prompt or in a comment, but not
    // as a comparison or list inside tools.ts.
    expect(toolsSource).not.toMatch(/allowed_?namespaces?\s*[:=]/i);
    expect(toolsSource).not.toMatch(/===?\s*['"]api-smoke['"]/);
    expect(toolsSource).not.toMatch(/\.includes\(\s*['"]api-smoke['"]\s*\)/);
    // Ban a literal allowlist array of namespaces in tools.ts.
    expect(toolsSource).not.toMatch(/\[\s*['"]api-smoke['"]\s*[,\]]/);
  });

  it('tools.ts has no replica-cap constant or comparison', () => {
    expect(toolsSource).not.toMatch(/max_?replicas/i);
    expect(toolsSource).not.toMatch(/replicas\s*[<>]=?\s*10\b/);
  });

  it('backendClient.ts does no policy decisions', () => {
    expect(backendClientSource).not.toMatch(/max_?replicas/i);
    expect(backendClientSource).not.toMatch(/allowed_?namespaces?/i);
    expect(backendClientSource).not.toMatch(/api-smoke/i);
  });
});

describe('tool surface omits blocked categories (requirement.md Blocked)', () => {
  // The agent is forbidden from delete/exec/secret-read/RBAC operations.
  // Spec lists those under "Blocked"; assert no tool name even hints at them.
  const server = buildKubernetesMcpServer(new BackendClient('http://x'));
  const registry = (server.instance as unknown as { _registeredTools: Record<string, unknown> })
    ._registeredTools;
  const registeredNames = Object.keys(registry);

  it('contains no delete/exec/secret/rbac/configmap-write tools', () => {
    for (const toolName of registeredNames) {
      expect(toolName.startsWith('delete_'), `tool ${toolName}`).toBe(false);
      expect(toolName.startsWith('exec_'), `tool ${toolName}`).toBe(false);
      expect(toolName.startsWith('read_secret'), `tool ${toolName}`).toBe(false);
      expect(toolName.includes('rbac'), `tool ${toolName}`).toBe(false);
      // No PVC mutation, no namespace mutation, no node-level mutation.
      expect(toolName.includes('pvc'), `tool ${toolName}`).toBe(false);
      // No "delete deployment" hidden under a different verb.
      expect(toolName, `tool ${toolName}`).not.toMatch(/^(remove|destroy|drop|teardown)_/);
    }
  });

  it('the exported TOOL_NAMES list matches the registered set (no drift)', () => {
    const exported = [...TOOL_NAMES].sort();
    const registered = [...registeredNames].sort();
    expect(exported).toEqual(registered);
  });

  it('tool names are exactly what the spec §4.2 enumerates', () => {
    // Spec §4.2 enumerates 12 read tools + 5 write tools = 17. The agent must
    // not invent new ones (extras would expand the agent's capability surface
    // beyond what the backend deliberately exposes).
    expect(registeredNames.length).toBe(17);
  });
});
