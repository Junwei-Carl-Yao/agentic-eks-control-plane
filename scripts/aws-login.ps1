# Refresh AWS login credentials and configure Terraform-compatible profile auth
# in the current PowerShell session so Terraform / awscli / kubectl can refresh
# creds during long-running operations.
#
# Must be dot-sourced - running normally sets the env vars in a child
# process that exits immediately:
#
#     . .\scripts\aws-login.ps1
#     . .\scripts\aws-login.ps1 -Profile prod -Region us-west-2

param(
    [string]$Profile = "dev",
    [string]$Region  = "us-east-1"
)

aws login --profile $Profile

# Build a temporary AWS config with a credential_process profile because
# Terraform's AWS SDK does not read the newer login_session profile format.
$awsConfigPath = Join-Path $env:USERPROFILE ".aws\config"
if (-not (Test-Path -Path $awsConfigPath)) {
    throw "AWS config not found at $awsConfigPath"
}

$terraformProfile = "$Profile-terraform"
$terraformConfigPath = Join-Path $env:TEMP "aws-config-$terraformProfile"
Get-Content -Path $awsConfigPath | Set-Content -Path $terraformConfigPath
Add-Content -Path $terraformConfigPath -Value @"

[profile $terraformProfile]
region = $Region
credential_process = aws configure export-credentials --profile $Profile
"@

# Use the temporary config + Terraform profile in this shell.
$env:AWS_CONFIG_FILE = $terraformConfigPath
$env:AWS_PROFILE = $terraformProfile
$env:AWS_REGION = $Region
$env:AWS_DEFAULT_REGION = $Region
$env:AWS_SDK_LOAD_CONFIG = "1"

# Clear static session credentials if previously exported.
Remove-Item Env:AWS_ACCESS_KEY_ID,Env:AWS_SECRET_ACCESS_KEY,Env:AWS_SESSION_TOKEN -ErrorAction SilentlyContinue
