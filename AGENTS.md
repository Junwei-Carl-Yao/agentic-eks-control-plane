# Project Prompt

## AWS Login Behavior

- Do not auto-run `.\scripts\aws-login.ps1` as part of normal command execution.
- After one login attempt, reuse the existing shell session credentials/profile and do not re-run the script before each Terraform/AWS command.
- Prefer non-interactive verification commands (for example `aws sts get-caller-identity`) before deciding login is needed.

## Default to removing, not adding

When a doc or code change is needed, prefer **deleting** the offending line over rewriting it with extra justification. If a line is already correct under the new design, leave it alone. If it's wrong or redundant, delete it - don't replace it with a longer version that re-explains the new state. The diff and surrounding context already convey the change. Only add new prose when the new design introduces a concept the reader cannot infer from what remains.

## Use descriptive variable names

All variables must have descriptive names - no single letters and no abbreviations. This applies to loop indices, range variables, short-lived locals, and receivers. Write `index`, `cluster`, `request`, `container`, `namespace`, `deployment`, `timestamp`, `replicaSet` - not `i`, `c`, `r`, `ct`, `ns`, `dep`, `ts`, `rs`.

The only exceptions are:
- single-letter names that are an established idiom for a well-known mathematical or domain convention (e.g., `x`/`y` for coordinates)
- the Go testing idioms `t *testing.T` and `b *testing.B`
- the Go conventions `ctx` for `context.Context`, `err` for `error`, and `ok` for the boolean second return of map/channel/type-assertion expressions
- package import identifiers (e.g., `metav1`, `appsv1`)
- the established Go shorthands `ops`, `deps`, `msg`, `cmd`, `args` (and capitalized/compound forms like `Ops`, `Deps`, `lastArgs`)

## Separate test agent for implementations

For every implementation request, structure work as two separate agents:

1. **Implementor agent** — performs the implementation.
2. **Unit test agent** — spawned fresh *after* the implementor finishes, with NO shared context from the implementor. It receives only the original user prompt / requirements, the relevant `implementation.md` (or equivalent spec), and pointers to the code that was written. It writes bias-free unit tests that verify both the stated requirements and the actual code correctness/functionality.

If the same agent (or a test agent sharing the implementor's context) writes the tests, it inherits the implementor's assumptions and tends to test what the code *does* rather than what the spec *requires*. The test agent's prompt must tell it to derive expected behavior from the spec/prompt first, then verify the code matches — not the other way around. Do not pass the implementor's transcript or reasoning into the test agent's prompt.
