import { vi, type MockInstance } from 'vitest';
import { AxiosError, AxiosHeaders } from 'axios';

import { apiClient } from '@/api/client';

// Shared per-route mock for apiClient.get used across ZoneMap integration
// tests. Mocks at the axios layer (not the clusterApi wrapper) so the real
// ApiError translation of 403 denials is still exercised — that translation
// is what surfaces guardrail reasons in the UI per the §5.3 invariant. ApiError
// checks `error instanceof AxiosError`, so rejections MUST be real AxiosError
// instances (a plain Error with isAxiosError set is silently treated as a
// generic error and the denial banner never appears).

export interface RouterHandlers {
  [url: string]: (params?: Record<string, string>) => unknown | AxiosError;
}

export function makeAxiosError(status: number, data: unknown, message: string): AxiosError {
  const headers = new AxiosHeaders();
  return new AxiosError(message, String(status), { headers } as never, null, {
    status,
    statusText: '',
    headers,
    config: { headers } as never,
    data,
  });
}

export function denialError(message: string): AxiosError {
  return makeAxiosError(
    403,
    {
      error: message,
      decision: { allow: false, action: 'list_pods', subject: 'ns/foo', reason: message },
    },
    message,
  );
}

export function genericError(status: number, message: string): AxiosError {
  return makeAxiosError(status, { error: message }, message);
}

export function mockRouter(handlers: RouterHandlers): MockInstance {
  return vi.spyOn(apiClient, 'get').mockImplementation((url: string, config?: unknown) => {
    const params = (config as { params?: Record<string, string> } | undefined)?.params;
    const handler = handlers[url];
    if (!handler) {
      return Promise.reject(genericError(500, `unmocked URL ${url}`));
    }
    const result = handler(params);
    if (result instanceof AxiosError) {
      return Promise.reject(result);
    }
    return Promise.resolve({ data: result });
  });
}
