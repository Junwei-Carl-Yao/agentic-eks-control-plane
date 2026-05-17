# Agent Runtime

Stateless TypeScript service that turns natural-language operator messages
into bounded Kubernetes operations against the Phase 2/3 backend. Uses the
Claude Agent SDK with a custom in-process MCP server whose tools wrap every
backend HTTP route. The backend's guardrail enforcer remains the only policy
chokepoint; this runtime never short-circuits a deny.

## Local development

1. `npm install`
2. Copy `.env.example` to `.env`. Set `BACKEND_URL=http://localhost:8000` and
   either `ANTHROPIC_API_KEY=...` directly, or leave both unset to fall back
   to `C:\Users\carly\Downloads\anthropic.txt`.
3. `npm run dev` starts the runtime on port `8081`. `curl
   http://localhost:8081/health` should return `{"status":"ok"}`.

The runtime is **stateless** — the frontend owns the conversation transcript
and resends it on every turn.

## Wire contract

`POST /api/agent/chat` (Content-Type `application/json`). Request body:

```json
{
  "transcript": [{"role": "user" | "assistant", "content": "..."}],
  "message": "..."
}
```

Response is `text/event-stream`. Each `data:` frame is a JSON object whose
`type` is one of:

- `tool_call` — `{type, id, tool, input}` when the agent invokes a tool.
- `tool_result` — `{type, id, ok, result, error}` after the tool returns. On
  a backend deny, `ok` is false and `error` carries the deny reason.
- `text` — `{type, delta}` assistant text.
- `done` — terminal frame; the server then closes the response.
- `error` — `{type, message}` fatal error; the stream closes after.

A heartbeat comment (`: ping`) is sent every 15 seconds so proxies do not
drop the connection.

## Eval harness

`npm run evals` boots an in-process mock backend (mirrors the real guardrail
shape), points the runtime at it, and runs each scenario in `test/evals/dataset.ts`
through the real Anthropic API. The harness writes `test/evals/report.json`
and exits non-zero on any failure. Run a single scenario with
`npm run evals -- --filter scale-allowed`.
