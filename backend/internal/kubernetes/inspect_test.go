package kubernetes

import (
	"context"
	"strconv"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func replicaCount(t *testing.T, client *Client, namespace, name string) int32 {
	t.Helper()
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deployment.Spec.Replicas == nil {
		return 0
	}
	return *deployment.Spec.Replicas
}

func isPaused(t *testing.T, client *Client, namespace, name string) bool {
	t.Helper()
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	return deployment.Spec.Paused
}

func currentRevision(t *testing.T, client *Client, namespace, name string) int64 {
	t.Helper()
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	annotationValue := deployment.Annotations[revisionAnnotation]
	if annotationValue == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(annotationValue, 10, 64)
	if err != nil {
		t.Fatalf("revision annotation %q not int: %v", annotationValue, err)
	}
	return parsed
}

func podTemplateAnnotation(t *testing.T, client *Client, namespace, name, key string) string {
	t.Helper()
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	return deployment.Spec.Template.Annotations[key]
}

func containerEnv(t *testing.T, client *Client, namespace, name, container string) map[string]string {
	t.Helper()
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	for _, containerSpec := range deployment.Spec.Template.Spec.Containers {
		if containerSpec.Name != container {
			continue
		}
		envMap := make(map[string]string, len(containerSpec.Env))
		for _, envVar := range containerSpec.Env {
			envMap[envVar.Name] = envVar.Value
		}
		return envMap
	}
	t.Fatalf("container %q not found", container)
	return nil
}

func hasEnvFrom(t *testing.T, client *Client, namespace, name, container, configMap string) bool {
	t.Helper()
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	for _, containerSpec := range deployment.Spec.Template.Spec.Containers {
		if containerSpec.Name != container {
			continue
		}
		for _, envFromSource := range containerSpec.EnvFrom {
			if envFromSource.ConfigMapRef != nil && envFromSource.ConfigMapRef.Name == configMap {
				return true
			}
		}
	}
	return false
}

func descending(events []Event) bool {
	for index := 1; index < len(events); index++ {
		if events[index-1].Time.Before(events[index].Time) {
			return false
		}
	}
	return true
}
