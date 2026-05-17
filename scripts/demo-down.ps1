param(
    [string]$ClusterName = "eks-ctrl",
    [int]$BackendPort = 8000,
    [int]$AgentPort = 8081,
    [int]$FrontendPort = 5173,
    [switch]$KeepCluster
)

$ErrorActionPreference = "Continue"

$repoRoot = Split-Path -Parent $PSScriptRoot
$runtimeDir = Join-Path $repoRoot ".demo"
$stateFile = Join-Path $runtimeDir "state.json"

$state = $null
if (Test-Path $stateFile) {
    try { $state = Get-Content $stateFile -Raw | ConvertFrom-Json } catch { $state = $null }
}

function Stop-PidIfAlive {
    param([int]$ProcessId, [string]$Label)
    if ($ProcessId -le 0) { return }
    try {
        Stop-Process -Id $ProcessId -Force -ErrorAction Stop
        Write-Host "    stopped $Label (PID $ProcessId)"
    } catch {
        # Process already gone or not ours; fall through to port-based cleanup.
    }
}

function Get-KindClusters {
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

function Stop-PortListener {
    param([int]$Port, [string]$Label)
    $connections = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    foreach ($connection in $connections) {
        try {
            Stop-Process -Id $connection.OwningProcess -Force -ErrorAction Stop
            Write-Host "    killed $Label PID $($connection.OwningProcess) on :$Port"
        } catch {
            Write-Host "    failed to kill PID $($connection.OwningProcess) on :$Port - $($_.Exception.Message)"
        }
    }
}

Write-Host "==> stopping services"
if ($state) {
    Stop-PidIfAlive -ProcessId $state.frontendPid -Label "frontend"
    Stop-PidIfAlive -ProcessId $state.agentPid    -Label "agent"
    Stop-PidIfAlive -ProcessId $state.backendPid  -Label "backend"
    if ($state.ports) {
        $BackendPort  = [int]$state.ports.backend
        $AgentPort    = [int]$state.ports.agent
        $FrontendPort = [int]$state.ports.frontend
    }
    if ($state.clusterName) { $ClusterName = $state.clusterName }
}

# npm/go often spawn children that outlive the parent on Windows — sweep the
# listening ports too. Both passes are idempotent.
Stop-PortListener -Port $FrontendPort -Label "frontend"
Stop-PortListener -Port $AgentPort    -Label "agent"
Stop-PortListener -Port $BackendPort  -Label "backend"

if (-not $KeepCluster) {
    Write-Host "==> deleting kind cluster $ClusterName"
    $clusters = Get-KindClusters
    if ($clusters -contains $ClusterName) {
        & kind delete cluster --name $ClusterName | Out-Null
    } else {
        Write-Host "    cluster not present"
    }
} else {
    Write-Host "==> keeping kind cluster $ClusterName (--KeepCluster)"
}

if (Test-Path $stateFile) { Remove-Item $stateFile -Force }

Write-Host ""
Write-Host "Teardown complete."
if (Test-Path $runtimeDir) {
    Write-Host "Logs preserved at $runtimeDir (delete manually if you want them gone)."
}
