package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// restartedAtAnnotation is the same key kubectl writes for `rollout restart`.
	// Using it lets existing tooling (kubectl rollout status / history) treat our
	// restart as a normal rollout.
	restartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"

	// revisionAnnotation matches the standard Deployment revision key. We track
	// the *current* revision on the Deployment for parity with the kubectl
	// rollout-history view; ReplicaSets carry the same key in production.
	revisionAnnotation = "deployment.kubernetes.io/revision"
)

// Scale sets a deployment's replica count. Replicas must be >= 1; scale-to-zero
// is rejected here so no caller (route, agent, future internal use) can stop a
// workload accidentally.
func (client *Client) Scale(ctx context.Context, namespace, name string, replicas int32) error {
	if replicas < 1 {
		return fmt.Errorf("kubernetes: scale: replicas must be >= 1, got %d", replicas)
	}
	return client.updateDeployment(ctx, namespace, name, func(deployment *appsv1.Deployment) {
		deployment.Spec.Replicas = &replicas
	})
}

// RolloutRestart patches the pod-template restartedAt annotation, triggering a
// rolling restart that downstream tooling recognises.
func (client *Client) RolloutRestart(ctx context.Context, namespace, name string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return client.updateDeployment(ctx, namespace, name, func(deployment *appsv1.Deployment) {
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = map[string]string{}
		}
		deployment.Spec.Template.Annotations[restartedAtAnnotation] = now
	})
}

// PauseRollout sets spec.paused = true. Subsequent edits queue rather than roll.
func (client *Client) PauseRollout(ctx context.Context, namespace, name string) error {
	return client.updateDeployment(ctx, namespace, name, func(deployment *appsv1.Deployment) {
		deployment.Spec.Paused = true
	})
}

// ResumeRollout clears spec.paused, allowing queued changes to roll out.
func (client *Client) ResumeRollout(ctx context.Context, namespace, name string) error {
	return client.updateDeployment(ctx, namespace, name, func(deployment *appsv1.Deployment) {
		deployment.Spec.Paused = false
	})
}

// Rollback reverts a Deployment to a prior revision. revision == 0 means
// "previous" (matches `kubectl rollout undo`); otherwise the explicit revision.
func (client *Client) Rollback(ctx context.Context, namespace, name string, revision int64) error {
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: deployment %s/%s", ErrNotFound, namespace, name)
		}
		return fmt.Errorf("kubernetes: get deployment: %w", err)
	}

	target, err := client.targetRevision(ctx, namespace, name, deployment, revision)
	if err != nil {
		return err
	}

	replicaSetList, err := client.kubernetesInterface.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("kubernetes: list replicasets: %w", err)
	}
	deploymentReplicaSets := replicaSetsForDeployment(replicaSetList.Items, deployment.Name)
	replicaSet := findReplicaSetByRevision(deploymentReplicaSets, target)
	if replicaSet == nil {
		return fmt.Errorf("kubernetes: rollback: revision %d not found for deployment %s/%s", target, namespace, name)
	}
	deployment.Spec.Template = replicaSet.Spec.Template
	if deployment.Annotations == nil {
		deployment.Annotations = map[string]string{}
	}
	deployment.Annotations[revisionAnnotation] = strconv.FormatInt(target, 10)

	if _, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("kubernetes: update deployment: %w", err)
	}
	return nil
}

// updateDeployment loads, mutates, and writes a Deployment. Centralising the
// load/save bookends keeps the operations small and the not-found handling
// consistent.
func (client *Client) updateDeployment(ctx context.Context, namespace, name string, mutate func(*appsv1.Deployment)) error {
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: deployment %s/%s", ErrNotFound, namespace, name)
		}
		return fmt.Errorf("kubernetes: get deployment: %w", err)
	}
	mutate(deployment)
	if _, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("kubernetes: update deployment: %w", err)
	}
	return nil
}

// targetRevision resolves a Rollback's target. revision > 0 is taken verbatim;
// revision == 0 picks the immediate predecessor of whatever revision the
// deployment currently sits on.
func (client *Client) targetRevision(ctx context.Context, namespace, name string, deployment *appsv1.Deployment, revision int64) (int64, error) {
	if revision > 0 {
		return revision, nil
	}
	current := parseRevision(deployment.Annotations[revisionAnnotation])
	replicaSetList, err := client.kubernetesInterface.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("kubernetes: list replicasets: %w", err)
	}
	revisions := collectRevisions(replicaSetsForDeployment(replicaSetList.Items, deployment.Name))
	if len(revisions) == 0 {
		return 0, errors.New("kubernetes: rollback: no revision history")
	}
	sort.Slice(revisions, func(left, right int) bool { return revisions[left] < revisions[right] })
	// Find the largest revision strictly less than current.
	previous := int64(0)
	for _, historicalRevision := range revisions {
		if historicalRevision < current {
			previous = historicalRevision
		}
	}
	if previous == 0 {
		// No predecessor found; fall back to the second-to-last entry.
		if len(revisions) >= 2 {
			previous = revisions[len(revisions)-2]
		} else {
			previous = revisions[0]
		}
	}
	return previous, nil
}

func replicaSetsForDeployment(items []appsv1.ReplicaSet, deploymentName string) []appsv1.ReplicaSet {
	filtered := make([]appsv1.ReplicaSet, 0, len(items))
	for _, replicaSet := range items {
		for _, owner := range replicaSet.OwnerReferences {
			if owner.Kind == "Deployment" && owner.Name == deploymentName {
				filtered = append(filtered, replicaSet)
				break
			}
		}
	}
	return filtered
}

func findReplicaSetByRevision(items []appsv1.ReplicaSet, revision int64) *appsv1.ReplicaSet {
	for index := range items {
		if parseRevision(items[index].Annotations[revisionAnnotation]) == revision {
			return &items[index]
		}
	}
	return nil
}

func collectRevisions(items []appsv1.ReplicaSet) []int64 {
	revisions := make([]int64, 0, len(items))
	for _, replicaSet := range items {
		if revision := parseRevision(replicaSet.Annotations[revisionAnnotation]); revision > 0 {
			revisions = append(revisions, revision)
		}
	}
	return revisions
}

func parseRevision(raw string) int64 {
	if raw == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
