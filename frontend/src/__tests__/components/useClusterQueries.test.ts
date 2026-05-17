import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import path from 'node:path';

// Spec (§5.3): "Cluster panel polls the backend read routes every 5 seconds
// via react-query". We assert the polling interval directly on the hook
// definitions — every cluster query must pass refetchInterval: 5000.

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

  it('every useQuery passes refetchInterval: 5000 (the §5.3 5s poll interval)', () => {
    const useQueryBlocks = hookSource.match(/useQuery\s*\(\s*\{[\s\S]*?\}\s*\)/g) ?? [];
    expect(useQueryBlocks.length).toBeGreaterThanOrEqual(6);
    for (const block of useQueryBlocks) {
      expect(block).toMatch(/refetchInterval:\s*(?:5000|POLL_INTERVAL_MS)/);
    }
  });

  it('the named POLL_INTERVAL_MS constant resolves to 5000', () => {
    const constant = hookSource.match(/POLL_INTERVAL_MS\s*=\s*(\d+)/);
    expect(constant).not.toBeNull();
    expect(Number(constant![1])).toBe(5000);
  });
});
