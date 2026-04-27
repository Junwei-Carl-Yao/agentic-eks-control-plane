# End-to-end Phase 1 teardown: destroy Terraform-managed infra, then scan
# AWS for orphans (the bootstrap S3 bucket is intentionally left in place).
#
# Usage:
#   .\scripts\teardown-all.ps1
#   $env:TF_ENV = "staging"; .\scripts\teardown-all.ps1

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

Invoke-Step "[1/2] make destroy"         "destroy"
Invoke-Step "[2/2] make teardown-verify" "teardown-verify"

Write-Host ""
Write-Host "Teardown complete."
