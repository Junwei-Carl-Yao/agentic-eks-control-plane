# Frontend

React + Vite + TypeScript dashboard for the Agentic EKS Control Plane.

## Layout

Single page, two panes:

- Left: cluster panel (deployments, pods, services, events, nodes) polled every 5s via react-query.
- Right: chat panel that POSTs to the agent runtime and renders the SSE stream live.

Code:

- `src/components/` — UI: `ClusterPanel`, `ChatPanel`, sections, chat bubbles.
- `src/api/client.ts` — Axios + typed read wrappers for the backend.
- `src/hooks/` — `useClusterQueries` (react-query), `useAgentChat` (SSE streaming + transcript state).
- `src/sse/parseSse.ts` — Line-buffered SSE parser used by `useAgentChat`.
- `src/types/` — TS types mirroring the Go DTOs in `backend/internal/kubernetes/types.go`.

## Dev quick-start

```
npm install
npm run dev    # Vite on http://localhost:5173
```

Vite proxies:

- `/api/agent/*` -> `http://localhost:8081` (agent runtime)
- `/api/*` -> `http://localhost:8000` (backend)

To exercise the full path locally you also need:

- The backend on `:8000` (`make dev-backend`) with a working `KUBECONFIG`.
- The agent runtime on `:8081` (`cd agent && npm run dev`) with `ANTHROPIC_API_KEY` set.
- A reachable kind cluster (or any kube context the backend can read).

## Scripts

- `npm run dev` — start Vite dev server.
- `npm run build` — typecheck + production build.
- `npm run typecheck` — `tsc --noEmit`.
- `npm run lint` — ESLint over `src/`.
- `npm run format` / `npm run format:check` — Prettier.
