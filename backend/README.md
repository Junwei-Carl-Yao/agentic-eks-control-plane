# Backend

Go API service that hosts the guardrailed execution layer.

## Layout

- `cmd/server/` — `main` entrypoint
- `internal/server/` — HTTP routes, CORS, middleware
- `internal/config/` — env-loaded settings
- `internal/logging/` — structured JSON logging (slog)

## Running locally

```
go run ./cmd/server
```

Override the listen address with `ADDR` (default `:8000`). Other env vars: `CORS_ORIGINS` (comma-separated), `AWS_REGION`, `KUBECONFIG`.
