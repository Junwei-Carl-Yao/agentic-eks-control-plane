package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ListDeployments returns all Deployments in a namespace. Empty namespace
// yields an empty slice with no error — listing an empty namespace is valid,
// only single-resource lookups should surface NotFound.
func (client *Client) ListDeployments(ctx context.Context, namespace string) ([]Deployment, error) {
	list, err := client.kubernetesInterface.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list deployments: %w", err)
	}
	deployments := make([]Deployment, 0, len(list.Items))
	for index := range list.Items {
		deployments = append(deployments, deploymentDTO(&list.Items[index]))
	}
	return deployments, nil
}

// GetDeployment returns the named deployment. Missing → IsNotFound.
func (client *Client) GetDeployment(ctx context.Context, namespace, name string) (Deployment, error) {
	deployment, err := client.kubernetesInterface.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Deployment{}, fmt.Errorf("%w: deployment %s/%s", ErrNotFound, namespace, name)
		}
		return Deployment{}, fmt.Errorf("kubernetes: get deployment: %w", err)
	}
	return deploymentDTO(deployment), nil
}

// ListPods returns pods in the namespace, optionally filtered by labelSelector.
// Empty selector returns every pod.
func (client *Client) ListPods(ctx context.Context, namespace, labelSelector string) ([]Pod, error) {
	list, err := client.kubernetesInterface.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list pods: %w", err)
	}
	pods := make([]Pod, 0, len(list.Items))
	for index := range list.Items {
		pods = append(pods, podDTO(&list.Items[index]))
	}
	return pods, nil
}

// ListEvents returns events for a namespace, ordered newest-first so the UI's
// "recent events" panel doesn't have to re-sort.
func (client *Client) ListEvents(ctx context.Context, namespace string) ([]Event, error) {
	list, err := client.kubernetesInterface.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list events: %w", err)
	}
	events := make([]Event, 0, len(list.Items))
	for index := range list.Items {
		events = append(events, eventDTO(&list.Items[index]))
	}
	sort.SliceStable(events, func(left, right int) bool { return events[left].Time.After(events[right].Time) })
	return events, nil
}

// TailLogs returns the last `lines` lines of the named container's log.
func (client *Client) TailLogs(ctx context.Context, namespace, pod, container string, lines int64) (string, error) {
	return client.logs.Tail(ctx, namespace, pod, container, lines)
}

// logSource is the seam separating the production log path (real GetLogs
// stream) from the in-memory test fixture.
type logSource interface {
	Tail(ctx context.Context, namespace, pod, container string, lines int64) (string, error)
}

type kubeLogSource struct {
	kubernetesInterface kubernetes.Interface
}

func (source *kubeLogSource) Tail(ctx context.Context, namespace, pod, container string, lines int64) (string, error) {
	options := &corev1.PodLogOptions{Container: container, TailLines: &lines}
	request := source.kubernetesInterface.CoreV1().Pods(namespace).GetLogs(pod, options)
	stream, err := request.Stream(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%w: pod %s/%s", ErrNotFound, namespace, pod)
		}
		return "", fmt.Errorf("kubernetes: stream logs: %w", err)
	}
	defer stream.Close()
	body, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("kubernetes: read logs: %w", err)
	}
	return string(body), nil
}

// tailLines returns the last n newline-separated lines of body. A trailing
// newline does not count as an extra line.
func tailLines(body string, lineLimit int64) string {
	if lineLimit <= 0 || body == "" {
		return body
	}
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if int64(len(allLines)) <= lineLimit {
		return strings.Join(allLines, "\n")
	}
	return strings.Join(allLines[int64(len(allLines))-lineLimit:], "\n")
}

func deploymentDTO(deployment *appsv1.Deployment) Deployment {
	paused := deployment.Spec.Paused
	var replicas int32
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	return Deployment{
		Name:              deployment.Name,
		Namespace:         deployment.Namespace,
		Replicas:          replicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		UpdatedReplicas:   deployment.Status.UpdatedReplicas,
		Paused:            paused,
	}
}

func podDTO(pod *corev1.Pod) Pod {
	return Pod{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Phase:     string(pod.Status.Phase),
		Labels:    pod.Labels,
	}
}

func eventDTO(event *corev1.Event) Event {
	eventTime := event.LastTimestamp.Time
	if eventTime.IsZero() {
		eventTime = event.EventTime.Time
	}
	if eventTime.IsZero() {
		eventTime = event.FirstTimestamp.Time
	}
	return Event{
		Namespace: event.Namespace,
		Reason:    event.Reason,
		Message:   event.Message,
		Type:      event.Type,
		Time:      eventTime,
		Object:    event.InvolvedObject.Name,
	}
}
