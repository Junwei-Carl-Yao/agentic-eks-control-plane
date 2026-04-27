# End-to-end Phase 1 apply: bootstrap remote state, apply Terraform,
# then verify the deployed infra (apply-verify also asserts cluster
# reachability and Ready nodes, so kubectl get nodes is redundant here).
#
# Usage:
#   .\scripts\apply-all.ps1
#   $env:TF_ENV = "staging"; .\scripts\apply-all.ps1

$ErrorActionPreference = "Stop"

Set-Location (Join-Path $PSScriptRoot "..")

function Invoke-Step {
    param([string]$Label, [string]$Target)
    Write-Host "==> $Label"
    & make $Target
    if ($LASTEXITCODE -ne 0) {
        throw "make $Target failed with exit code $LASTEXITCODE"
    }
}

Invoke-Step "[1/3] make bootstrap"    "bootstrap"
Invoke-Step "[2/3] make apply"        "apply"
Invoke-Step "[3/3] make apply-verify" "apply-verify"

Write-Host ""
Write-Host "Apply complete."
