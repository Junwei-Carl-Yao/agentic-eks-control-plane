/* eslint-disable react-refresh/only-export-components */
import type { ReactElement, ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';

// freshClient builds a QueryClient with retry disabled so failed queries
// surface immediately in tests instead of hammering the mocked fetch three
// times.
export function freshClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, refetchOnWindowFocus: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
}

interface ProvidersProps {
  client: QueryClient;
  children: ReactNode;
}

function Providers({ client, children }: ProvidersProps) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

export function renderWithClient(ui: ReactElement, client: QueryClient = freshClient()) {
  const utils = render(<Providers client={client}>{ui}</Providers>);
  return { ...utils, client };
}

// makeSseResponse builds a fetch-style Response whose body is a ReadableStream
// that yields the given chunks one at a time. Lets us drive the chat panel
// with realistic chunked SSE without hitting the network.
export function makeSseResponse(chunks: string[], init?: ResponseInit): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
    ...init,
  });
}

// makeSseResponseFromQueue lets a test push chunks asynchronously so we can
// observe intermediate UI states (e.g. input disabled while streaming).
export interface SseQueue {
  response: Response;
  push: (chunk: string) => void;
  end: () => void;
  abort: (reason?: unknown) => void;
}

export function makeSseResponseQueue(): SseQueue {
  const encoder = new TextEncoder();
  let controllerRef: ReadableStreamDefaultController<Uint8Array> | null = null;
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controllerRef = controller;
    },
  });
  const response = new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  });
  return {
    response,
    push(chunk: string) {
      controllerRef?.enqueue(encoder.encode(chunk));
    },
    end() {
      controllerRef?.close();
    },
    abort(reason) {
      controllerRef?.error(reason);
    },
  };
}

// jsonResponse helps mock cluster fetches: clusterApi uses axios, which calls
// fetch only when adapter is overridden. We instead spy on axios via the api
// client; see tests for usage. This helper is for SSE-style fetch mocks only.
export function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
}
