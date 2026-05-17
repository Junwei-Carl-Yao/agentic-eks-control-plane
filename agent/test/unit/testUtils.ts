// Shared helpers for unit tests. Keeps the per-test files focused on
// assertions rather than scaffolding.

import { vi } from 'vitest';

export interface RecordedFetch {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: unknown;
}

export interface MockFetchHandle {
  recorded: RecordedFetch[];
  restore(): void;
}

interface MockResponseSpec {
  status?: number;
  body?: unknown;
}

// installMockFetch replaces global.fetch with a function that records each
// call and returns the configured response. The same response is returned for
// every call until restore() is invoked.
export function installMockFetch(
  response: MockResponseSpec = { status: 200, body: {} },
): MockFetchHandle {
  const recorded: RecordedFetch[] = [];
  const original = globalThis.fetch;

  const fakeFetch = vi.fn(async (input: unknown, init?: unknown) => {
    const url = typeof input === 'string' ? input : String(input);
    const initObject = (init ?? {}) as {
      method?: string;
      headers?: Record<string, string>;
      body?: unknown;
    };
    let parsedBody: unknown = null;
    if (typeof initObject.body === 'string' && initObject.body.length > 0) {
      try {
        parsedBody = JSON.parse(initObject.body);
      } catch {
        parsedBody = initObject.body;
      }
    }
    recorded.push({
      url,
      method: initObject.method ?? 'GET',
      headers: initObject.headers ?? {},
      body: parsedBody,
    });
    const status = response.status ?? 200;
    const bodyText = JSON.stringify(response.body ?? {});
    return new Response(bodyText, {
      status,
      headers: { 'Content-Type': 'application/json' },
    });
  });

  // @ts-expect-error overwrite for tests
  globalThis.fetch = fakeFetch;

  return {
    recorded,
    restore() {
      // @ts-expect-error restore original
      globalThis.fetch = original;
    },
  };
}

// parseUrl returns the path, query map, and a normalized search string for an
// absolute URL produced by BackendClient.
export function parseUrl(url: string): { path: string; query: Record<string, string> } {
  const parsed = new URL(url);
  const query: Record<string, string> = {};
  parsed.searchParams.forEach((value, key) => {
    query[key] = value;
  });
  return { path: parsed.pathname, query };
}
