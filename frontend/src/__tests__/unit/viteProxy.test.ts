import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import path from 'node:path';

// Spec (Phase 5.1, §5.3, fixed contracts):
// - /api/agent/* proxies to http://localhost:8081 (agent runtime SSE)
// - /api/* proxies to http://localhost:8000 (backend)
// - /api/agent must be matched BEFORE /api so the SSE call is not swallowed
//   by the backend proxy. Vite matches in declaration order, so we assert the
//   key order on server.proxy as well as the targets.
//
// We parse vite.config.ts as text rather than importing it: dynamic-importing
// vite from inside a jsdom test triggers esbuild's TextEncoder check and fails
// before we can inspect anything.

const viteConfigSource = readFileSync(path.resolve(__dirname, '../../../vite.config.ts'), 'utf8');

describe('vite proxy configuration', () => {
  it('proxies /api/agent to the agent runtime on :8081', () => {
    // The order of properties matters in Vite proxy resolution. We extract the
    // proxy block and locate each rule by its key.
    const agentMatch = viteConfigSource.match(
      /['"]\/api\/agent['"]:\s*\{[^}]*target:\s*['"]([^'"]+)['"]/,
    );
    expect(agentMatch).not.toBeNull();
    expect(agentMatch![1]).toBe('http://localhost:8081');
  });

  it('proxies /api to the backend on :8000', () => {
    // Match the bare `/api` key (not `/api/agent`).
    const apiMatch = viteConfigSource.match(/['"]\/api['"]:\s*\{[^}]*target:\s*['"]([^'"]+)['"]/);
    expect(apiMatch).not.toBeNull();
    expect(apiMatch![1]).toBe('http://localhost:8000');
  });

  it('declares /api/agent BEFORE /api so the more specific prefix wins', () => {
    const agentIndex = viteConfigSource.indexOf("'/api/agent'");
    const apiIndex = viteConfigSource.indexOf("'/api'", agentIndex + 1);
    // Agent key must appear first; the next occurrence of '/api' must come
    // strictly after.
    expect(agentIndex).toBeGreaterThanOrEqual(0);
    expect(apiIndex).toBeGreaterThan(agentIndex);
  });
});
