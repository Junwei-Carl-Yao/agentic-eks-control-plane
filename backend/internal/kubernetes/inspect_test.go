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

func configMapData(t *testing.T, client *Client, namespace, name string) map[string]string {
	t.Helper()
	configMap, err := client.kubernetesInterface.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	return configMap.Data
}

func configMapHasBinaryDataKey(t *testing.T, client *Client, namespace, name, key string) bool {
	t.Helper()
	configMap, err := client.kubernetesInterface.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	_, ok := configMap.BinaryData[key]
	return ok
}

func descending(events []Event) bool {
	for index := 1; index < len(events); index++ {
		if events[index-1].Time.Before(events[index].Time) {
			return false
		}
	}
	return true
}
