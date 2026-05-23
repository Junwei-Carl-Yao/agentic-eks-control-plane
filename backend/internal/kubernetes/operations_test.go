// Phase 2.2 — mutation operations on Deployments.
// These tests assert *what the operation does to the resource*, not whether policy
// allows it. Policy enforcement is Phase 3.
package kubernetes

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
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

// Scenario: ResolveRollbackImage picks the container whose Name matches the
// Deployment name, not the first container in declaration order. With
// `istio-proxy` declared before `app` in a Deployment named `app`, the
// resolver must return `app`'s image — picking the first would let a sidecar
// image leak into the floor check and bypass the guardrail.
func TestResolveRollbackImage_PicksMatchingNameContainer(t *testing.T) {
	previousContainers := []corev1.Container{
		{Name: "istio-proxy", Image: "istio/proxyv2:v9"},
		{Name: "app", Image: "repo/app:v4"},
	}
	currentContainers := []corev1.Container{
		{Name: "istio-proxy", Image: "istio/proxyv2:v9"},
		{Name: "app", Image: "repo/app:v6"},
	}
	kubeClient := newFakeClient(t, withRevisionHistoryAndContainers("app", "app", []revisionContainers{
		{revision: 1, containers: previousContainers},
		{revision: 2, containers: currentContainers},
	}))
	// revision == 0 chooses the predecessor (revision 1 here), whose `app`
	// container is `repo/app:v4`.
	image, err := kubeClient.ResolveRollbackImage(context.Background(), "app", "app", 0)
	if err != nil {
		t.Fatalf("ResolveRollbackImage: %v", err)
	}
	if image != "repo/app:v4" {
		t.Errorf("image = %q, want %q (matching-name container, not the first)", image, "repo/app:v4")
	}
}

// Scenario: no container in the target ReplicaSet has the Deployment's name →
// fall back to the first container. Required for legacy Deployments where the
// container name does not echo the Deployment.
func TestResolveRollbackImage_FallbackToFirstContainer(t *testing.T) {
	previousContainers := []corev1.Container{
		{Name: "primary", Image: "repo/primary:v3"},
		{Name: "sidecar", Image: "repo/sidecar:v9"},
	}
	currentContainers := []corev1.Container{
		{Name: "primary", Image: "repo/primary:v5"},
		{Name: "sidecar", Image: "repo/sidecar:v9"},
	}
	// Deployment name "app" does not match any container name.
	kubeClient := newFakeClient(t, withRevisionHistoryAndContainers("app", "app", []revisionContainers{
		{revision: 1, containers: previousContainers},
		{revision: 2, containers: currentContainers},
	}))
	image, err := kubeClient.ResolveRollbackImage(context.Background(), "app", "app", 0)
	if err != nil {
		t.Fatalf("ResolveRollbackImage: %v", err)
	}
	if image != "repo/primary:v3" {
		t.Errorf("image = %q, want %q (first container, no name match)", image, "repo/primary:v3")
	}
}

// Scenario: the target ReplicaSet has zero containers → error. The enforcer
// surfaces this as the "could not resolve target image" deny, so the route
// returns 403 instead of attempting a rollback against a broken revision.
func TestResolveRollbackImage_ZeroContainersReturnsError(t *testing.T) {
	currentContainers := []corev1.Container{{Name: "app", Image: "repo/app:v6"}}
	kubeClient := newFakeClient(t, withRevisionHistoryAndContainers("app", "app", []revisionContainers{
		{revision: 1, containers: nil},
		{revision: 2, containers: currentContainers},
	}))
	if _, err := kubeClient.ResolveRollbackImage(context.Background(), "app", "app", 1); err == nil {
		t.Fatal("ResolveRollbackImage: expected error for zero-container revision, got nil")
	}
}

// Scenario: revision == 0 chooses the predecessor (parallel to Rollback's
// "previous" semantics). With current=3, the prior is 2, so the v2 image must
// come back — not the v3 image and not v1.
func TestResolveRollbackImage_RevisionZeroChoosesPredecessor(t *testing.T) {
	kubeClient := newFakeClient(t, withRevisionHistoryAndContainers("app", "app", []revisionContainers{
		{revision: 1, containers: []corev1.Container{{Name: "app", Image: "repo/app:v1"}}},
		{revision: 2, containers: []corev1.Container{{Name: "app", Image: "repo/app:v2"}}},
		{revision: 3, containers: []corev1.Container{{Name: "app", Image: "repo/app:v3"}}},
	}))
	image, err := kubeClient.ResolveRollbackImage(context.Background(), "app", "app", 0)
	if err != nil {
		t.Fatalf("ResolveRollbackImage: %v", err)
	}
	if image != "repo/app:v2" {
		t.Errorf("image = %q, want %q (predecessor of current revision 3)", image, "repo/app:v2")
	}
}

// Scenario: explicit revision returns that specific revision's container
// image, not a neighbouring one. Pins the lookup by revision number.
func TestResolveRollbackImage_ExplicitRevision(t *testing.T) {
	kubeClient := newFakeClient(t, withRevisionHistoryAndContainers("app", "app", []revisionContainers{
		{revision: 1, containers: []corev1.Container{{Name: "app", Image: "repo/app:v1"}}},
		{revision: 2, containers: []corev1.Container{{Name: "app", Image: "repo/app:v2"}}},
		{revision: 3, containers: []corev1.Container{{Name: "app", Image: "repo/app:v3"}}},
	}))
	image, err := kubeClient.ResolveRollbackImage(context.Background(), "app", "app", 1)
	if err != nil {
		t.Fatalf("ResolveRollbackImage: %v", err)
	}
	if image != "repo/app:v1" {
		t.Errorf("image = %q, want %q (explicit revision 1)", image, "repo/app:v1")
	}
}

// Scenario: Deployment does not exist → IsNotFound. The enforcer wraps the
// underlying error in its "could not resolve target image" deny; that wrapping
// preserves IsNotFound through errors.Is for any downstream caller still
// classifying. We assert on IsNotFound directly to avoid coupling to wrapping.
func TestResolveRollbackImage_NotFound(t *testing.T) {
	kubeClient := newFakeClient(t)
	_, err := kubeClient.ResolveRollbackImage(context.Background(), "app", "ghost", 0)
	if err == nil {
		t.Fatal("ResolveRollbackImage on missing deployment: expected error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("err = %v, want IsNotFound", err)
	}
}
