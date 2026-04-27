# Project Prompt

## AWS Login Behavior

- Do not auto-run `.\scripts\aws-login.ps1` as part of normal command execution.
- After one login attempt, reuse the existing shell session credentials/profile and do not re-run the script before each Terraform/AWS command.
- Prefer non-interactive verification commands (for example `aws sts get-caller-identity`) before deciding login is needed.
