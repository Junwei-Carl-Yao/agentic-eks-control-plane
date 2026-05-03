param(
    [string]$ClusterName = "ekscp-local",
    [string]$Namespace = "api-smoke",
    [string]$BackendDir = "backend",
    [string]$BaseUrl = "http://localhost:8000"
)

$ErrorActionPreference = "Stop"

function Assert-CommandExists {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command not found: $Name"
    }
}

function Invoke-External {
    param(
        [ScriptBlock]$Command,
        [string]$ErrorMessage
    )
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$ErrorMessage (exit code $LASTEXITCODE)"
    }
}

function Invoke-Check {
    param(
        [string]$Name,
        [string]$Method,
        [string]$Url,
        [string]$BodyFile = ""
    )

    $curlArgs = @("-sS", "-o", "NUL", "-w", "%{http_code}", "-X", $Method)
    if ($BodyFile -ne "") {
        $curlArgs += @("-H", "Content-Type: application/json", "--data-binary", "@$BodyFile")
    }
    $curlArgs += $Url

    $status = (& curl.exe @curlArgs).Trim()
    if ($status -ne "200") {
        throw "Check failed: $Name returned HTTP $status"
    }
    Write-Output "[PASS] $Name (HTTP 200)"
}

function Invoke-ApiJson {
    param(
        [string]$Name,
        [string]$Method,
        [string]$Url,
        [string]$BodyFile = ""
    )

    $tmpBody = [System.IO.Path]::GetTempFileName()
    try {
        $curlArgs = @("-sS", "-o", $tmpBody, "-w", "%{http_code}", "-X", $Method)
        if ($BodyFile -ne "") {
            $curlArgs += @("-H", "Content-Type: application/json", "--data-binary", "@$BodyFile")
        }
        $curlArgs += $Url

        $status = (& curl.exe @curlArgs).Trim()
        if ($status -ne "200") {
            $raw = Get-Content $tmpBody -Raw
            throw "Check failed: $Name returned HTTP $status. Body: $raw"
        }
        $rawBody = Get-Content $tmpBody -Raw
        Write-Output "[PASS] $Name (HTTP 200)"
        if ([string]::IsNullOrWhiteSpace($rawBody)) {
            return $null
        }
        return $rawBody | ConvertFrom-Json
    }
    finally {
        Remove-Item $tmpBody -Force -ErrorAction SilentlyContinue
    }
}

function Wait-Until {
    param(
        [ScriptBlock]$Condition,
        [string]$Description,
        [int]$TimeoutSeconds = 120,
        [int]$IntervalSeconds = 2
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if (& $Condition) {
            return
        }
        Start-Sleep -Seconds $IntervalSeconds
    }
    throw "Timeout waiting for: $Description"
}

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

function Get-Text {
    param([object]$Value)
    if ($null -eq $Value) {
        return ""
    }
    return ([string]$Value).Trim()
}

function Get-DeploymentJson {
    param(
        [string]$NamespaceName,
        [string]$DeploymentName
    )
    $json = & kubectl -n $NamespaceName get deployment $DeploymentName -o json
    if ($LASTEXITCODE -ne 0) {
        return $null
    }
    return ($json | ConvertFrom-Json)
}

Assert-CommandExists "kind"
Assert-CommandExists "kubectl"
Assert-CommandExists "curl.exe"
Assert-CommandExists "go"

$repoRoot = (Resolve-Path ".").Path
$backendPath = Join-Path $repoRoot $BackendDir
$tmpDir = Join-Path $backendPath "tmp-smoke"
$runId = (Get-Date -Format "yyyyMMddHHmmss")
$outLog = Join-Path $backendPath "server.$runId.out.log"
$errLog = Join-Path $backendPath "server.$runId.err.log"
$backendProc = $null

try {
    Write-Output "Cleaning up any existing kind cluster '$ClusterName'..."
    Invoke-External -Command { kind delete cluster --name $ClusterName | Out-Null } -ErrorMessage "Failed to delete existing kind cluster"

    Write-Output "Creating kind cluster '$ClusterName'..."
    Invoke-External -Command { kind create cluster --name $ClusterName --wait 180s | Out-Null } -ErrorMessage "Failed to create kind cluster"

    Write-Output "Preparing test workload in namespace '$Namespace'..."
    Invoke-External -Command { (& kubectl create namespace $Namespace --dry-run=client -o yaml) | & kubectl apply -f - | Out-Null } -ErrorMessage "Failed to create namespace"
    Invoke-External -Command { (& kubectl -n $Namespace create deployment demo-nginx --image=nginx --replicas=2 --dry-run=client -o yaml) | & kubectl apply -f - | Out-Null } -ErrorMessage "Failed to create deployment"
    Invoke-External -Command { (& kubectl -n $Namespace expose deployment demo-nginx --port=80 --target-port=80 --dry-run=client -o yaml) | & kubectl apply -f - | Out-Null } -ErrorMessage "Failed to expose deployment"
    Invoke-External -Command { kubectl -n $Namespace rollout status deployment/demo-nginx --timeout=120s | Out-Null } -ErrorMessage "Deployment did not roll out"

    New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null

    Write-Output "Starting backend..."
    Get-Process -Name "server" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    $env:KUBECONFIG = Join-Path $HOME ".kube\config"
    $backendProc = Start-Process -FilePath "go" -ArgumentList "run ./cmd/server" -WorkingDirectory $backendPath -WindowStyle Hidden -PassThru -RedirectStandardOutput $outLog -RedirectStandardError $errLog
    Start-Sleep -Seconds 3
    if ($backendProc.HasExited) {
        $errText = ""
        if (Test-Path $errLog) { $errText = Get-Content $errLog -Raw }
        throw "Backend failed to start. $errText"
    }
    Write-Output "Waiting for backend health endpoint..."
    try {
        Wait-Until -Description "backend health endpoint" -TimeoutSeconds 90 -IntervalSeconds 2 -Condition {
            $status = (& curl.exe -sS -o NUL -w "%{http_code}" "$BaseUrl/health" 2>$null).Trim()
            return $status -eq "200"
        } | Out-Null
    }
    catch {
        $errText = ""
        $outText = ""
        if (Test-Path $errLog) { $errText = Get-Content $errLog -Raw }
        if (Test-Path $outLog) { $outText = Get-Content $outLog -Raw }
        throw "Backend did not become healthy in time. stderr:`n$errText`nstdout:`n$outText"
    }

    $pod = Get-Text (& kubectl -n $Namespace get pods -l app=demo-nginx -o jsonpath='{.items[0].metadata.name}')
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to query pod name (exit code $LASTEXITCODE)"
    }
    if ([string]::IsNullOrWhiteSpace($pod)) {
        throw "Unable to resolve demo-nginx pod name."
    }

    $scaleFile = Join-Path $tmpDir "scale.json"
    $pauseFile = Join-Path $tmpDir "pause.json"
    $rollbackFile = Join-Path $tmpDir "rollback.json"
    $updateEnvFile = Join-Path $tmpDir "update-env.json"

    Set-Content -Path $scaleFile -Value (@{ namespace = $Namespace; name = "demo-nginx"; replicas = 5 } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $pauseFile -Value (@{ namespace = $Namespace; name = "demo-nginx" } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $rollbackFile -Value (@{ namespace = $Namespace; name = "demo-nginx"; revision = 1 } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $updateEnvFile -Value (@{ namespace = $Namespace; name = "demo-nginx"; container = "nginx"; env = @{ EXAMPLE_FLAG = "true" } } | ConvertTo-Json -Compress) -NoNewline

    Write-Output "Running endpoint checks (Terraform endpoints skipped)..."
    $health = Invoke-ApiJson -Name "health" -Method "GET" -Url "$BaseUrl/health"
    Assert-True ($health.status -eq "ok") "Health endpoint did not return status=ok."

    $deployments = Invoke-ApiJson -Name "cluster-deployments" -Method "GET" -Url "$BaseUrl/api/cluster/deployments?namespace=$Namespace"
    Assert-True (($deployments | Where-Object { $_.name -eq "demo-nginx" }).Count -ge 1) "Deployments endpoint did not include demo-nginx."

    $deployment = Invoke-ApiJson -Name "cluster-deployment-by-name" -Method "GET" -Url "$BaseUrl/api/cluster/deployments/demo-nginx?namespace=$Namespace"
    Assert-True ($deployment.name -eq "demo-nginx") "Deployment-by-name endpoint returned wrong deployment."
    Assert-True ($deployment.namespace -eq $Namespace) "Deployment-by-name endpoint returned wrong namespace."

    $pods = Invoke-ApiJson -Name "cluster-pods" -Method "GET" -Url "$BaseUrl/api/cluster/pods?namespace=$Namespace&labelSelector=app%3Ddemo-nginx"
    Assert-True (($pods | Measure-Object).Count -ge 1) "Pods endpoint returned no pods for demo-nginx selector."
    Assert-True (($pods | Where-Object { $_.phase -eq "Running" } | Measure-Object).Count -ge 1) "Pods endpoint did not include running pods."

    $events = Invoke-ApiJson -Name "cluster-events" -Method "GET" -Url "$BaseUrl/api/cluster/events?namespace=$Namespace"
    Assert-True (($events | Measure-Object).Count -ge 1) "Events endpoint returned no events."

    $logs = Invoke-ApiJson -Name "cluster-logs" -Method "GET" -Url "$BaseUrl/api/cluster/logs?namespace=$Namespace&pod=$pod&container=nginx&lines=5"
    Assert-True (-not [string]::IsNullOrWhiteSpace($logs.logs)) "Logs endpoint returned empty logs."

    $restartBefore = Get-Text (& kubectl -n $Namespace get deployment demo-nginx -o jsonpath='{.spec.template.metadata.annotations.kubectl\.kubernetes\.io/restartedAt}')

    Invoke-Check -Name "op-scale" -Method "POST" -Url "$BaseUrl/api/operations/scale" -BodyFile $scaleFile
    Invoke-External -Command { kubectl -n $Namespace rollout status deployment/demo-nginx --timeout=180s | Out-Null } -ErrorMessage "Scale rollout did not complete"
    Wait-Until -Description "scaled deployment to 5 replicas" -Condition {
        $d = (& kubectl -n $Namespace get deployment demo-nginx -o jsonpath='{.status.availableReplicas} {.spec.replicas}')
        if ($LASTEXITCODE -ne 0) { return $false }
        $parts = $d.Trim().Split(" ", [System.StringSplitOptions]::RemoveEmptyEntries)
        return ($parts.Count -eq 2 -and $parts[0] -eq "5" -and $parts[1] -eq "5")
    } | Out-Null
    $scaledPods = (& kubectl -n $Namespace get pods -l app=demo-nginx --field-selector=status.phase=Running --no-headers 2>$null | Measure-Object -Line).Lines
    Assert-True ($scaledPods -eq 5) "Scale operation did not result in 5 running pods."

    $scaledDeployment = Invoke-ApiJson -Name "verify-deployment-after-scale" -Method "GET" -Url "$BaseUrl/api/cluster/deployments/demo-nginx?namespace=$Namespace"
    Assert-True ($scaledDeployment.replicas -eq 5) "Deployment read endpoint did not report replicas=5 after scale."
    Assert-True ($scaledDeployment.availableReplicas -eq 5) "Deployment read endpoint did not report availableReplicas=5 after scale."

    Invoke-Check -Name "op-pause-rollout" -Method "POST" -Url "$BaseUrl/api/operations/pause-rollout" -BodyFile $pauseFile
    Wait-Until -Description "deployment paused" -Condition {
        $paused = Get-Text (& kubectl -n $Namespace get deployment demo-nginx -o jsonpath='{.spec.paused}')
        return $LASTEXITCODE -eq 0 -and $paused -eq "true"
    } | Out-Null

    Invoke-Check -Name "op-resume-rollout" -Method "POST" -Url "$BaseUrl/api/operations/resume-rollout" -BodyFile $pauseFile
    Wait-Until -Description "deployment resumed" -Condition {
        $paused = Get-Text (& kubectl -n $Namespace get deployment demo-nginx -o jsonpath='{.spec.paused}')
        return $LASTEXITCODE -eq 0 -and ($paused -eq "" -or $paused -eq "false")
    } | Out-Null

    Invoke-Check -Name "op-rollout-restart" -Method "POST" -Url "$BaseUrl/api/operations/rollout-restart" -BodyFile $pauseFile
    Invoke-External -Command { kubectl -n $Namespace rollout status deployment/demo-nginx --timeout=180s | Out-Null } -ErrorMessage "Rollout restart did not complete"
    Wait-Until -Description "restart annotation updated" -Condition {
        $restartAfter = Get-Text (& kubectl -n $Namespace get deployment demo-nginx -o jsonpath='{.spec.template.metadata.annotations.kubectl\.kubernetes\.io/restartedAt}')
        return $LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($restartAfter) -and $restartAfter -ne $restartBefore
    } | Out-Null

    Invoke-Check -Name "op-update-env" -Method "POST" -Url "$BaseUrl/api/operations/update-env" -BodyFile $updateEnvFile
    Invoke-External -Command { kubectl -n $Namespace rollout status deployment/demo-nginx --timeout=180s | Out-Null } -ErrorMessage "Update-env rollout did not complete"
    Wait-Until -Description "EXAMPLE_FLAG env var applied" -Condition {
        $dep = Get-DeploymentJson -NamespaceName $Namespace -DeploymentName "demo-nginx"
        if ($null -eq $dep) { return $false }
        $container = $dep.spec.template.spec.containers | Where-Object { $_.name -eq "nginx" } | Select-Object -First 1
        if ($null -eq $container) { return $false }
        $envVar = $container.env | Where-Object { $_.name -eq "EXAMPLE_FLAG" } | Select-Object -First 1
        return ($null -ne $envVar -and $envVar.value -eq "true")
    } | Out-Null

    Invoke-Check -Name "op-rollback" -Method "POST" -Url "$BaseUrl/api/operations/rollback" -BodyFile $rollbackFile
    Invoke-External -Command { kubectl -n $Namespace rollout status deployment/demo-nginx --timeout=180s | Out-Null } -ErrorMessage "Rollback rollout did not complete"
    Wait-Until -Description "rollback removed EXAMPLE_FLAG from revision 1" -Condition {
        $dep = Get-DeploymentJson -NamespaceName $Namespace -DeploymentName "demo-nginx"
        if ($null -eq $dep) { return $false }
        $container = $dep.spec.template.spec.containers | Where-Object { $_.name -eq "nginx" } | Select-Object -First 1
        if ($null -eq $container) { return $false }
        $envVar = $container.env | Where-Object { $_.name -eq "EXAMPLE_FLAG" } | Select-Object -First 1
        return ($null -eq $envVar)
    } | Out-Null

    Write-Output "All backend endpoint checks passed."
}
finally {
    Write-Output "Starting teardown..."
    if ($backendProc -and -not $backendProc.HasExited) {
        Stop-Process -Id $backendProc.Id -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $tmpDir) {
        Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
    }
    Get-ChildItem -Path $backendPath -Filter "server.*.out.log" -File -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
    Get-ChildItem -Path $backendPath -Filter "server.*.err.log" -File -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
    & kubectl delete namespace $Namespace --ignore-not-found=true --wait=true | Out-Null
    & kind delete cluster --name $ClusterName | Out-Null
    Write-Output "Teardown complete."
}
