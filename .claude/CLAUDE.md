# Project conventions

## Default to removing, not adding

When a doc or code change is needed, prefer **deleting** the offending line over rewriting it with extra justification. If a line is already correct under the new design, leave it alone. If it's wrong or redundant, delete it — don't replace it with a longer version that re-explains the new state. The diff and surrounding context already convey the change. Only add new prose when the new design introduces a concept the reader cannot infer from what remains.
