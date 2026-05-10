param(
    [string]$ClusterName = "ekscp-local",
    [string]$Namespace = "api-smoke",
    [string]$BackendDir = "backend",
    [string]$BaseUrl = "http://localhost:8000"
)

$ErrorActionPreference = "Stop"

# Phase 3 policy is hardcoded in the backend (see internal/guardrails/policy.go).
# Mirror the same literals here so this script seeds matching cluster state.
# MAX_REPLICAS is sourced from the feature-flag ConfigMap at runtime, and is
# the only key on the FeatureFlagKeys allowlist — the update-feature-flag
# allow-path therefore writes to it directly. NonAllowlistedKey is seeded into
# app-flags to prove the GetFeatureFlags route narrows its response.
$FeatureFlagConfigMap = "app-flags"
$FeatureFlagKey = "MAX_REPLICAS"
$NonAllowlistedKey = "SECRET_TOKEN"
$MaxReplicasPolicy = 5
$MaxReplicasUpdated = 8

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
        [string]$BodyFile = "",
        [int]$ExpectStatus = 200
    )

    $curlArgs = @("-sS", "-o", "NUL", "-w", "%{http_code}", "-X", $Method)
    if ($BodyFile -ne "") {
        $curlArgs += @("-H", "Content-Type: application/json", "--data-binary", "@$BodyFile")
    }
    $curlArgs += $Url

    $status = (& curl.exe @curlArgs).Trim()
    if ($status -ne [string]$ExpectStatus) {
        throw "Check failed: $Name returned HTTP $status, expected $ExpectStatus"
    }
    Write-Host "[PASS] $Name (HTTP $status)"
}

function Invoke-ApiJson {
    param(
        [string]$Name,
        [string]$Method,
        [string]$Url,
        [string]$BodyFile = "",
        [int]$ExpectStatus = 200
    )

    $tmpBody = [System.IO.Path]::GetTempFileName()
    try {
        $curlArgs = @("-sS", "-o", $tmpBody, "-w", "%{http_code}", "-X", $Method)
        if ($BodyFile -ne "") {
            $curlArgs += @("-H", "Content-Type: application/json", "--data-binary", "@$BodyFile")
        }
        $curlArgs += $Url

        $status = (& curl.exe @curlArgs).Trim()
        if ($status -ne [string]$ExpectStatus) {
            $raw = Get-Content $tmpBody -Raw
            throw "Check failed: $Name returned HTTP $status, expected $ExpectStatus. Body: $raw"
        }
        $rawBody = Get-Content $tmpBody -Raw
        Write-Host "[PASS] $Name (HTTP $status)"
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

    Write-Output "Seeding allowlisted ConfigMap '$FeatureFlagConfigMap' (with MAX_REPLICAS=$MaxReplicasPolicy and a non-allowlisted '$NonAllowlistedKey' to verify GetFeatureFlags narrowing) and a non-allowlisted ConfigMap for deny tests..."
    Invoke-External -Command { (& kubectl -n $Namespace create configmap $FeatureFlagConfigMap --from-literal=MAX_REPLICAS=$MaxReplicasPolicy --from-literal=$NonAllowlistedKey=leak-me --dry-run=client -o yaml) | & kubectl apply -f - | Out-Null } -ErrorMessage "Failed to seed feature-flag ConfigMap"
    Invoke-External -Command { (& kubectl -n $Namespace create configmap other-config --from-literal=anything=value --dry-run=client -o yaml) | & kubectl apply -f - | Out-Null } -ErrorMessage "Failed to seed non-allowlisted ConfigMap"

    New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null

    Write-Output "Starting backend (guardrail policy is hardcoded; MAX_REPLICAS sourced from $FeatureFlagConfigMap)..."
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
    $featureFlagFile = Join-Path $tmpDir "feature-flag.json"

    # Allow-path bodies.
    Set-Content -Path $scaleFile -Value (@{ namespace = $Namespace; name = "demo-nginx"; replicas = $MaxReplicasPolicy } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $pauseFile -Value (@{ namespace = $Namespace; name = "demo-nginx" } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $rollbackFile -Value (@{ namespace = $Namespace; name = "demo-nginx"; revision = 1 } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $featureFlagFile -Value (@{ namespace = $Namespace; configmap = $FeatureFlagConfigMap; key = $FeatureFlagKey; value = [string]$MaxReplicasUpdated } | ConvertTo-Json -Compress) -NoNewline

    # Deny-path bodies. Each one targets a single guardrail rule so a regression
    # tells us exactly which check failed.
    $denyBlockedNamespaceFile = Join-Path $tmpDir "deny-blocked-namespace.json"
    $denyOtherNamespaceFile = Join-Path $tmpDir "deny-other-namespace.json"
    $denyOverMaxReplicasFile = Join-Path $tmpDir "deny-over-max.json"
    $denyOtherConfigMapFile = Join-Path $tmpDir "deny-other-configmap.json"
    $denyOtherKeyFile = Join-Path $tmpDir "deny-other-key.json"
    $denyInvalidNameFile = Join-Path $tmpDir "deny-invalid-name.json"

    Set-Content -Path $denyBlockedNamespaceFile -Value (@{ namespace = "kube-system"; name = "demo-nginx"; replicas = 1 } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $denyOtherNamespaceFile -Value (@{ namespace = "not-on-allowlist"; name = "demo-nginx"; replicas = 1 } | ConvertTo-Json -Compress) -NoNewline
    # Deny-over-max runs after the update step has tightened MAX_REPLICAS to
    # $MaxReplicasUpdated, so the deny target must exceed the *new* bound.
    Set-Content -Path $denyOverMaxReplicasFile -Value (@{ namespace = $Namespace; name = "demo-nginx"; replicas = ($MaxReplicasUpdated + 1) } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $denyOtherConfigMapFile -Value (@{ namespace = $Namespace; configmap = "other-config"; key = $FeatureFlagKey; value = "v" } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $denyOtherKeyFile -Value (@{ namespace = $Namespace; configmap = $FeatureFlagConfigMap; key = "NOT_ALLOWED"; value = "v" } | ConvertTo-Json -Compress) -NoNewline
    Set-Content -Path $denyInvalidNameFile -Value (@{ namespace = $Namespace; name = "INVALID_NAME"; replicas = 1 } | ConvertTo-Json -Compress) -NoNewline

    Write-Output "=== Read-only endpoint checks ==="
    $health = Invoke-ApiJson -Name "health" -Method "GET" -Url "$BaseUrl/health"
    Assert-True ($health.status -eq "ok") "Health endpoint did not return status=ok."

    $deployments = Invoke-ApiJson -Name "cluster-deployments" -Method "GET" -Url "$BaseUrl/api/cluster/deployments?namespace=$Namespace"
    Assert-True ((@($deployments | Where-Object { $_.name -eq "demo-nginx" })).Count -ge 1) "Deployments endpoint did not include demo-nginx."

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

    $services = Invoke-ApiJson -Name "cluster-services" -Method "GET" -Url "$BaseUrl/api/cluster/services?namespace=$Namespace"
    Assert-True ((@($services | Where-Object { $_.name -eq "demo-nginx" })).Count -ge 1) "Services endpoint did not include demo-nginx."

    Invoke-ApiJson -Name "cluster-ingresses" -Method "GET" -Url "$BaseUrl/api/cluster/ingresses?namespace=$Namespace" | Out-Null
    Invoke-ApiJson -Name "cluster-hpas" -Method "GET" -Url "$BaseUrl/api/cluster/hpas?namespace=$Namespace" | Out-Null

    $namespaces = Invoke-ApiJson -Name "cluster-namespaces" -Method "GET" -Url "$BaseUrl/api/cluster/namespaces"
    Assert-True ((@($namespaces | Where-Object { $_.name -eq $Namespace })).Count -ge 1) "Namespaces endpoint did not include $Namespace."

    $nodes = Invoke-ApiJson -Name "cluster-nodes" -Method "GET" -Url "$BaseUrl/api/cluster/nodes"
    Assert-True ((@($nodes)).Count -ge 1) "Nodes endpoint returned no nodes."
    # Phase 2.2 contract: no addresses, capacity, or labels are exposed. Check that
    # the projection truly only carries `name` so a future regression doesn't
    # widen the surface.
    foreach ($node in @($nodes)) {
        $nodeProperties = @($node.PSObject.Properties | Where-Object { $null -ne $_.Value } | ForEach-Object { $_.Name })
        Assert-True (($nodeProperties.Count -eq 1) -and ($nodeProperties[0] -eq "name")) "Nodes endpoint exposed extra fields: $($nodeProperties -join ',')"
    }

    # The feature-flags read returns a plain map[string]string narrowed to
    # FeatureFlagKeys. Seeded $NonAllowlistedKey must not appear in the response.
    $featureFlags = Invoke-ApiJson -Name "cluster-feature-flags" -Method "GET" -Url "$BaseUrl/api/cluster/feature-flags?namespace=$Namespace"
    Assert-True ($featureFlags.MAX_REPLICAS -eq [string]$MaxReplicasPolicy) "Feature-flags endpoint did not return MAX_REPLICAS=$MaxReplicasPolicy; got $($featureFlags.MAX_REPLICAS)."
    $featureFlagProperties = @($featureFlags.PSObject.Properties | ForEach-Object { $_.Name })
    Assert-True (-not ($featureFlagProperties -contains $NonAllowlistedKey)) "Feature-flags endpoint leaked non-allowlisted key ${NonAllowlistedKey}: $($featureFlagProperties -join ',')"

    $replicaSets = Invoke-ApiJson -Name "cluster-replicasets" -Method "GET" -Url "$BaseUrl/api/cluster/replicasets?namespace=$Namespace"
    Assert-True ((@($replicaSets | Where-Object { $_.owner -eq "demo-nginx" })).Count -ge 1) "ReplicaSets endpoint did not include any RS owned by demo-nginx."

    Write-Output "=== Mutation allow-path checks (Phase 3 enforcer must approve each) ==="
    $restartBefore = Get-Text (& kubectl -n $Namespace get deployment demo-nginx -o jsonpath='{.spec.template.metadata.annotations.kubectl\.kubernetes\.io/restartedAt}')

    Invoke-Check -Name "op-scale" -Method "POST" -Url "$BaseUrl/api/operations/scale" -BodyFile $scaleFile
    Invoke-External -Command { kubectl -n $Namespace rollout status deployment/demo-nginx --timeout=180s | Out-Null } -ErrorMessage "Scale rollout did not complete"
    Wait-Until -Description "scaled deployment to $MaxReplicasPolicy replicas" -Condition {
        $d = (& kubectl -n $Namespace get deployment demo-nginx -o jsonpath='{.status.availableReplicas} {.spec.replicas}')
        if ($LASTEXITCODE -ne 0) { return $false }
        $parts = $d.Trim().Split(" ", [System.StringSplitOptions]::RemoveEmptyEntries)
        return ($parts.Count -eq 2 -and $parts[0] -eq [string]$MaxReplicasPolicy -and $parts[1] -eq [string]$MaxReplicasPolicy)
    } | Out-Null
    $scaledPods = (& kubectl -n $Namespace get pods -l app=demo-nginx --field-selector=status.phase=Running --no-headers 2>$null | Measure-Object -Line).Lines
    Assert-True ($scaledPods -eq $MaxReplicasPolicy) "Scale operation did not result in $MaxReplicasPolicy running pods."

    $scaledDeployment = Invoke-ApiJson -Name "verify-deployment-after-scale" -Method "GET" -Url "$BaseUrl/api/cluster/deployments/demo-nginx?namespace=$Namespace"
    Assert-True ($scaledDeployment.replicas -eq $MaxReplicasPolicy) "Deployment read endpoint did not report replicas=$MaxReplicasPolicy after scale."
    Assert-True ($scaledDeployment.availableReplicas -eq $MaxReplicasPolicy) "Deployment read endpoint did not report availableReplicas=$MaxReplicasPolicy after scale."

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

    Invoke-Check -Name "op-update-feature-flag" -Method "POST" -Url "$BaseUrl/api/operations/update-feature-flag" -BodyFile $featureFlagFile
    Wait-Until -Description "$FeatureFlagKey applied to $FeatureFlagConfigMap" -Condition {
        $value = Get-Text (& kubectl -n $Namespace get configmap $FeatureFlagConfigMap -o jsonpath="{.data.$FeatureFlagKey}")
        return $LASTEXITCODE -eq 0 -and $value -eq [string]$MaxReplicasUpdated
    } | Out-Null
    $featureFlagsAfter = Invoke-ApiJson -Name "verify-feature-flags-after-update" -Method "GET" -Url "$BaseUrl/api/cluster/feature-flags?namespace=$Namespace"
    Assert-True ($featureFlagsAfter.MAX_REPLICAS -eq [string]$MaxReplicasUpdated) "Feature-flags read did not reflect updated MAX_REPLICAS=$MaxReplicasUpdated; got $($featureFlagsAfter.MAX_REPLICAS)."

    Invoke-Check -Name "op-rollback" -Method "POST" -Url "$BaseUrl/api/operations/rollback" -BodyFile $rollbackFile
    Invoke-External -Command { kubectl -n $Namespace rollout status deployment/demo-nginx --timeout=180s | Out-Null } -ErrorMessage "Rollback rollout did not complete"
    Wait-Until -Description "rollback cleared restartedAt annotation by reverting to revision 1" -Condition {
        $dep = Get-DeploymentJson -NamespaceName $Namespace -DeploymentName "demo-nginx"
        if ($null -eq $dep) { return $false }
        $annotations = $dep.spec.template.metadata.annotations
        if ($null -eq $annotations) { return $true }
        return ($null -eq $annotations.'kubectl.kubernetes.io/restartedAt' -or [string]::IsNullOrWhiteSpace($annotations.'kubectl.kubernetes.io/restartedAt'))
    } | Out-Null

    Write-Output "=== Mutation deny-path checks (Phase 3 enforcer must reject each with 403) ==="
    Invoke-Check -Name "deny-scale-blocked-namespace (kube-system)" -Method "POST" -Url "$BaseUrl/api/operations/scale" -BodyFile $denyBlockedNamespaceFile -ExpectStatus 403
    Invoke-Check -Name "deny-scale-namespace-not-on-allowlist" -Method "POST" -Url "$BaseUrl/api/operations/scale" -BodyFile $denyOtherNamespaceFile -ExpectStatus 403
    Invoke-Check -Name "deny-scale-over-MAX_REPLICAS" -Method "POST" -Url "$BaseUrl/api/operations/scale" -BodyFile $denyOverMaxReplicasFile -ExpectStatus 403
    Invoke-Check -Name "deny-update-feature-flag-non-allowlisted-configmap" -Method "POST" -Url "$BaseUrl/api/operations/update-feature-flag" -BodyFile $denyOtherConfigMapFile -ExpectStatus 403
    Invoke-Check -Name "deny-update-feature-flag-non-allowlisted-key" -Method "POST" -Url "$BaseUrl/api/operations/update-feature-flag" -BodyFile $denyOtherKeyFile -ExpectStatus 403
    # The model layer accepts non-empty names; DNS-1123 is the enforcer's check
    # (so the audit log records *why* the name was rejected). Hence 403, not 400.
    Invoke-Check -Name "deny-scale-invalid-DNS-1123-name" -Method "POST" -Url "$BaseUrl/api/operations/scale" -BodyFile $denyInvalidNameFile -ExpectStatus 403

    Write-Output "=== Verifying denied operations did not mutate cluster state ==="
    $denyState = Invoke-ApiJson -Name "verify-deployment-unchanged-after-denies" -Method "GET" -Url "$BaseUrl/api/cluster/deployments/demo-nginx?namespace=$Namespace"
    Assert-True ($denyState.replicas -eq $MaxReplicasPolicy) "Denied scales must not have changed replicas; got $($denyState.replicas)."

    $otherCM = Get-Text (& kubectl -n $Namespace get configmap other-config -o jsonpath="{.data.$FeatureFlagKey}")
    Assert-True ([string]::IsNullOrEmpty($otherCM)) "Denied feature-flag write must not have touched other-config."

    Write-Output "All backend endpoint checks passed."
}
finally {
    Write-Output "Starting teardown..."
    if ($backendProc -and -not $backendProc.HasExited) {
        Stop-Process -Id $backendProc.Id -Force -ErrorAction SilentlyContinue
    }
    # `go run` spawns a separate `server.exe` binary; killing the go parent on
    # Windows does not cascade. Always sweep both names so a follow-up run
    # doesn't bind to a stale port.
    Get-Process -Name "server", "go" -ErrorAction SilentlyContinue | Where-Object {
        $_.Path -and $_.Path.StartsWith($backendPath)
    } | Stop-Process -Force -ErrorAction SilentlyContinue
    Get-Process -Name "server" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    if (Test-Path $tmpDir) {
        Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
    }
    Get-ChildItem -Path $backendPath -Filter "server.*.out.log" -File -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
    Get-ChildItem -Path $backendPath -Filter "server.*.err.log" -File -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
    & kubectl delete namespace $Namespace --ignore-not-found=true --wait=true | Out-Null
    & kind delete cluster --name $ClusterName | Out-Null
    Write-Output "Teardown complete."
}
