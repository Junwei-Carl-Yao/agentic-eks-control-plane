import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import path from 'node:path';

// Spec (§5.3): "Cluster panel polls the backend read routes every 5 seconds
// via react-query". We assert the polling interval directly on the hook
// definitions — every cluster query that targets a per-tick read route must
// pass refetchInterval: 5000. The identity hook (useClusterIdentity) is the
// one explicit exception: cluster name/region don't change, so it's fetched
// once per session with staleTime: Infinity instead.

const hookSource = readFileSync(
  path.resolve(__dirname, '../../hooks/useClusterQueries.ts'),
  'utf8',
);

describe('useClusterQueries', () => {
  it('exports a hook for every section the cluster panel needs', () => {
    for (const hook of [
      'useNamespaces',
      'useNodes',
      'useDeployments',
      'usePods',
      'useServices',
      'useEvents',
    ]) {
      expect(hookSource).toMatch(new RegExp(`export\\s+function\\s+${hook}\\b`));
    }
  });

  it('every polling useQuery passes refetchInterval: 5000 (the §5.3 5s poll interval)', () => {
    const useQueryBlocks = hookSource.match(/useQuery\s*\(\s*\{[\s\S]*?\}\s*\)/g) ?? [];
    expect(useQueryBlocks.length).toBeGreaterThanOrEqual(6);
    for (const block of useQueryBlocks) {
      // Identity opts out by setting staleTime: Infinity instead of polling.
      if (/staleTime:\s*Infinity/.test(block)) continue;
      expect(block).toMatch(/refetchInterval:\s*(?:5000|POLL_INTERVAL_MS)/);
    }
  });

  it('the named POLL_INTERVAL_MS constant resolves to 5000', () => {
    const constant = hookSource.match(/POLL_INTERVAL_MS\s*=\s*(\d+)/);
    expect(constant).not.toBeNull();
    expect(Number(constant![1])).toBe(5000);
  });

  // useClusterIdentity is the "fetch once per session" hook — cluster name +
  // region are config-derived and never change. It must opt out of polling
  // (staleTime: Infinity) AND must not carry a refetchInterval that would
  // re-fire the request on a timer.
  it('useClusterIdentity is configured for a one-shot fetch (staleTime: Infinity, no refetchInterval)', () => {
    const identityBlock = extractHookBody('useClusterIdentity');
    expect(identityBlock).toMatch(/staleTime:\s*Infinity/);
    expect(identityBlock).not.toMatch(/refetchInterval/);
  });

  // useClusterHealth is the polling counterpart that drives the topbar dot.
  // It MUST poll at POLL_INTERVAL_MS so the UI sees apiserver outages within
  // a single tick.
  it('useClusterHealth polls at POLL_INTERVAL_MS', () => {
    const healthBlock = extractHookBody('useClusterHealth');
    expect(healthBlock).toMatch(/refetchInterval:\s*(?:POLL_INTERVAL_MS|5000)/);
    // And it must hit the new /api/cluster/health route, not /info.
    expect(healthBlock).toMatch(/clusterApi\.health\(\)/);
  });
});

// extractHookBody returns the full source of `export function <name>(...) { ... }`
// from useClusterQueries.ts. Used by the per-hook polling assertions so the
// match is scoped to the named hook rather than the whole file.
function extractHookBody(name: string): string {
  const start = hookSource.indexOf(`export function ${name}`);
  if (start < 0) {
    throw new Error(`hook ${name} not found in useClusterQueries.ts`);
  }
  // Walk forward from the opening `{` of the function body, tracking brace
  // depth, until we close the function. The hook bodies in this file are
  // small object literals plus a single useQuery call — brace counting is
  // sufficient here.
  const openBrace = hookSource.indexOf('{', start);
  let depth = 0;
  for (let cursor = openBrace; cursor < hookSource.length; cursor++) {
    const character = hookSource[cursor];
    if (character === '{') depth++;
    else if (character === '}') {
      depth--;
      if (depth === 0) {
        return hookSource.slice(start, cursor + 1);
      }
    }
  }
  throw new Error(`unbalanced braces while scanning ${name}`);
}
