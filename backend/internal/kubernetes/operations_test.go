// Phase 2.2 — mutation operations on Deployments.
// These tests assert *what the operation does to the resource*, not whether policy
// allows it. Policy enforcement is Phase 3.
package kubernetes

import (
	"context"
	"testing"
	"time"
)

// Scenario: Scale(ns, name, 5) → spec.replicas becomes 5.
func TestScale_SetsReplicas(t *testing.T) {
	kubeClient := newFakeClient(t, withDeploymentReplicas("app", "web", 1))
	if err := kubeClient.Scale(context.Background(), "app", "web", 5); err != nil {
		t.Fatalf("Scale: %v", err)
	}
	if currentReplicas := replicaCount(t, kubeClient, "app", "web"); currentReplicas != 5 {
		t.Errorf("replicas = %d, want 5", currentReplicas)
	}
}

// Scenario: replicas=0 is rejected by Scale itself. Scale-to-zero is effectively
// a "stop the workload" operation; we want it gated at the function so no caller
// (route, agent, future internal use) can trigger it accidentally.
func TestScale_ZeroIsRejected(t *testing.T) {
	kubeClient := newFakeClient(t, withDeploymentReplicas("app", "web", 3))
	if err := kubeClient.Scale(context.Background(), "app", "web", 0); err == nil {
		t.Fatal("Scale to 0: expected error, got nil")
	}
	if currentReplicas := replicaCount(t, kubeClient, "app", "web"); currentReplicas != 3 {
		t.Errorf("replicas mutated to %d despite rejection; want 3", currentReplicas)
	}
}

// Scenario: RolloutRestart patches the pod-template annotation kubectl uses,
// so existing tooling (kubectl rollout status, kubectl rollout history) treats it
// as a normal restart.
func TestRolloutRestart_PatchesRestartedAtAnnotation(t *testing.T) {
	kubeClient := newFakeClient(t, withDeployments("app", "web"))
	beforeRestart := time.Now().Add(-time.Second)
	if err := kubeClient.RolloutRestart(context.Background(), "app", "web"); err != nil {
		t.Fatalf("RolloutRestart: %v", err)
	}
	restartedAtAnnotation := podTemplateAnnotation(t, kubeClient, "app", "web", "kubectl.kubernetes.io/restartedAt")
	parsedTimestamp, err := time.Parse(time.RFC3339, restartedAtAnnotation)
	if err != nil {
		t.Fatalf("annotation %q not RFC3339: %v", restartedAtAnnotation, err)
	}
	if parsedTimestamp.Before(beforeRestart) {
		t.Errorf("restartedAt %v predates the call (%v)", parsedTimestamp, beforeRestart)
	}
}

// Scenario: PauseRollout sets spec.paused = true so subsequent edits queue rather than roll.
func TestPauseRollout_SetsPaused(t *testing.T) {
	kubeClient := newFakeClient(t, withDeployments("app", "web"))
	if err := kubeClient.PauseRollout(context.Background(), "app", "web"); err != nil {
		t.Fatalf("PauseRollout: %v", err)
	}
	if !isPaused(t, kubeClient, "app", "web") {
		t.Error("spec.paused = false, want true")
	}
}

// Scenario: ResumeRollout clears spec.paused so queued changes roll out.
func TestResumeRollout_ClearsPaused(t *testing.T) {
	kubeClient := newFakeClient(t, withPausedDeployment("app", "web"))
	if err := kubeClient.ResumeRollout(context.Background(), "app", "web"); err != nil {
		t.Fatalf("ResumeRollout: %v", err)
	}
	if isPaused(t, kubeClient, "app", "web") {
		t.Error("spec.paused still true after Resume")
	}
}

// Scenario: Rollback with no revision → reverts the deployment to the immediately
// previous ReplicaSet, mirroring `kubectl rollout undo`.
func TestRollback_DefaultsToPreviousRevision(t *testing.T) {
	kubeClient := newFakeClient(t, withRevisionHistory("app", "web", []int64{1, 2, 3}))
	if err := kubeClient.Rollback(context.Background(), "app", "web", 0); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if activeRevision := currentRevision(t, kubeClient, "app", "web"); activeRevision != 2 {
		t.Errorf("revision = %d, want 2", activeRevision)
	}
}

// Scenario: Rollback(ns, name, 1) → reverts to revision 1 specifically.
func TestRollback_ToSpecificRevision(t *testing.T) {
	kubeClient := newFakeClient(t, withRevisionHistory("app", "web", []int64{1, 2, 3}))
	if err := kubeClient.Rollback(context.Background(), "app", "web", 1); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if activeRevision := currentRevision(t, kubeClient, "app", "web"); activeRevision != 1 {
		t.Errorf("revision = %d, want 1", activeRevision)
	}
}

func TestRollback_SpecificRevisionMissingReturnsError(t *testing.T) {
	kubeClient := newFakeClient(t, withRevisionHistory("app", "web", []int64{1, 2, 3}))
	if err := kubeClient.Rollback(context.Background(), "app", "web", 99); err == nil {
		t.Fatal("Rollback missing revision: expected error, got nil")
	}
}

func TestRollback_IgnoresReplicaSetsFromOtherDeployments(t *testing.T) {
	kubeClient := newFakeClient(t,
		withRevisionHistory("app", "web", []int64{1, 2, 3}),
		withRevisionHistory("app", "api", []int64{1, 2, 9}),
	)
	if err := kubeClient.Rollback(context.Background(), "app", "web", 0); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if activeRevision := currentRevision(t, kubeClient, "app", "web"); activeRevision != 2 {
		t.Errorf("revision = %d, want 2", activeRevision)
	}
}

// Scenario: deployment missing → operation returns IsNotFound, not a generic error,
// so the route layer can map cleanly to 404.
func TestOperations_NotFoundIsTyped(t *testing.T) {
	kubeClient := newFakeClient(t)
	if err := kubeClient.Scale(context.Background(), "app", "ghost", 1); !IsNotFound(err) {
		t.Errorf("err = %v, want IsNotFound", err)
	}
}
