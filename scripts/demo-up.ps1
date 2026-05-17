param(
    [string]$ClusterName = "eks-ctrl",
    [string]$Namespace = "api-smoke",
    [int]$BackendPort = 8000,
    [int]$AgentPort = 8081,
    [int]$FrontendPort = 5173,
    [string]$AnthropicKeyFile = "$env:USERPROFILE\Downloads\anthropic.txt"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$runtimeDir = Join-Path $repoRoot ".demo"
New-Item -ItemType Directory -Force -Path $runtimeDir | Out-Null

function Require-Command {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command not found: $Name"
    }
}

function Test-Port {
    param([int]$Port)
    return [bool](Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
}

function Get-KindClusters {
    # kind writes "No kind clusters found." to stderr when none exist; under
    # ErrorActionPreference=Stop PowerShell 5.1 turns that into a terminating
    # error even with redirection. Run kind with stderr swallowed and the
    # preference relaxed for just this call.
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $stdout = & kind get clusters 2>$null
    } finally {
        $ErrorActionPreference = $previous
    }
    if (-not $stdout) { return @() }
    return @($stdout -split "`r?`n" | Where-Object { $_ -ne "" })
}

function Wait-Http {
    param([string]$Url, [int]$TimeoutSec = 60)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) { return }
        } catch {}
        Start-Sleep -Milliseconds 500
    }
    throw "Timed out waiting for $Url after ${TimeoutSec}s"
}

function Start-Service-Process {
    # Spawns the service via `cmd /c` so its stdio can redirect to log files
    # uniformly (Start-Process can't -RedirectStandardOutput a .cmd shim like
    # npm). Returns the PID of whatever ends up listening on $Port, which is
    # the real node/go process rather than the cmd.exe wrapper.
    param(
        [string]$Name,
        [int]$Port,
        [string]$HealthPath = "/health",
        [string]$WorkingDirectory,
        [string]$Command,
        [hashtable]$Environment,
        [int]$TimeoutSec = 90
    )
    $logFile = Join-Path $runtimeDir "$Name.log"
    $errFile = "$logFile.err"
    Remove-Item -Force -ErrorAction SilentlyContinue $logFile, $errFile
    foreach ($key in $Environment.Keys) { Set-Item "env:$key" $Environment[$key] }
    $cmdLine = "$Command > `"$logFile`" 2> `"$errFile`""
    Start-Process -FilePath "cmd.exe" -ArgumentList "/c", $cmdLine `
        -WorkingDirectory $WorkingDirectory -WindowStyle Hidden | Out-Null
    Wait-Http "http://localhost:$Port$HealthPath" -TimeoutSec $TimeoutSec
    $connection = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $connection) { throw "$Name not listening on :$Port after health check" }
    return [int]$connection.OwningProcess
}

Require-Command kind
Require-Command kubectl
Require-Command go
Require-Command npm

if (-not (Test-Path $AnthropicKeyFile)) {
    throw "Anthropic API key not found at $AnthropicKeyFile"
}

Write-Host "==> kind cluster: $ClusterName"
$clusters = Get-KindClusters
if ($clusters -contains $ClusterName) {
    Write-Host "    already exists, reusing"
} else {
    & kind create cluster --name $ClusterName | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "kind create cluster failed" }
}

$kubeContext = "kind-$ClusterName"
Write-Host "==> namespace + workloads in $Namespace"
& kubectl --context $kubeContext create namespace $Namespace 2>$null | Out-Null
& kubectl --context $kubeContext -n $Namespace create deployment web --image=nginx:1.27-alpine --replicas=2 2>$null | Out-Null
& kubectl --context $kubeContext -n $Namespace create deployment api --image=nginx:1.27-alpine --replicas=1 2>$null | Out-Null
& kubectl --context $kubeContext -n $Namespace expose deployment web --port=80 2>$null | Out-Null
& kubectl --context $kubeContext -n $Namespace rollout status deployment/web --timeout=60s | Out-Null
& kubectl --context $kubeContext -n $Namespace rollout status deployment/api --timeout=60s | Out-Null

if (Test-Port $BackendPort) { throw "Port $BackendPort already in use" }
if (Test-Port $AgentPort)   { throw "Port $AgentPort already in use" }
if (Test-Port $FrontendPort){ throw "Port $FrontendPort already in use" }

Write-Host "==> backend on :$BackendPort"
$backendPid = Start-Service-Process -Name "backend" -Port $BackendPort `
    -HealthPath "/health" `
    -WorkingDirectory (Join-Path $repoRoot "backend") `
    -Command "go run ./cmd/server" `
    -Environment @{
        KUBECONFIG = "$env:USERPROFILE\.kube\config"
        ADDR       = ":$BackendPort"
    } -TimeoutSec 120

Write-Host "==> agent runtime on :$AgentPort"
$agentPid = Start-Service-Process -Name "agent" -Port $AgentPort `
    -HealthPath "/health" `
    -WorkingDirectory (Join-Path $repoRoot "agent") `
    -Command "npm run dev" `
    -Environment @{
        PORT                    = "$AgentPort"
        BACKEND_URL             = "http://localhost:$BackendPort"
        ANTHROPIC_API_KEY_FILE  = $AnthropicKeyFile
    }

Write-Host "==> frontend on :$FrontendPort"
$frontendPid = Start-Service-Process -Name "frontend" -Port $FrontendPort `
    -HealthPath "/" `
    -WorkingDirectory (Join-Path $repoRoot "frontend") `
    -Command "npm run dev" -Environment @{}

$state = @{
    clusterName = $ClusterName
    backendPid  = $backendPid
    agentPid    = $agentPid
    frontendPid = $frontendPid
    ports       = @{ backend = $BackendPort; agent = $AgentPort; frontend = $FrontendPort }
}
$stateFile = Join-Path $runtimeDir "state.json"
$state | ConvertTo-Json | Set-Content -Path $stateFile -Encoding UTF8

Write-Host ""
Write-Host "Demo is up."
Write-Host "  Dashboard: http://localhost:$FrontendPort"
Write-Host "  Backend:   http://localhost:$BackendPort"
Write-Host "  Agent:     http://localhost:$AgentPort"
Write-Host "  Logs:      $runtimeDir\{backend,agent,frontend}.log"
Write-Host "  State:     $stateFile"
Write-Host ""
Write-Host "Tear down with: scripts\demo-down.ps1"
