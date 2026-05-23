package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// clusterHealthProbeTimeout caps the apiserver /livez probe so a hung
// control plane can't stall /api/cluster/info or /api/cluster/health. The
// handler returns Healthy = false when this fires.
const clusterHealthProbeTimeout = 3 * time.Second

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
// Empty selector returns every pod. CPU/memory usage is joined in from
// metrics-server (PodMetrics) and divided by the host node's allocatable so
// the UI gets the same 0..1 scale the node bars already use.
func (client *Client) ListPods(ctx context.Context, namespace, labelSelector string) ([]Pod, error) {
	list, err := client.kubernetesInterface.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list pods: %w", err)
	}
	podUsageByName := client.podMetricsByName(ctx, namespace)
	nodeAllocatableByName := client.nodeAllocatableByName(ctx)
	pods := make([]Pod, 0, len(list.Items))
	for index := range list.Items {
		pod := &list.Items[index]
		pods = append(pods, podDTO(pod, podUsageByName[pod.Name], nodeAllocatableByName[pod.Spec.NodeName]))
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

// ListServices returns Services in a namespace.
func (client *Client) ListServices(ctx context.Context, namespace string) ([]Service, error) {
	list, err := client.kubernetesInterface.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list services: %w", err)
	}
	services := make([]Service, 0, len(list.Items))
	for index := range list.Items {
		services = append(services, serviceDTO(&list.Items[index]))
	}
	return services, nil
}

// ListIngresses returns networking.k8s.io/v1 Ingresses in a namespace.
func (client *Client) ListIngresses(ctx context.Context, namespace string) ([]Ingress, error) {
	list, err := client.kubernetesInterface.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list ingresses: %w", err)
	}
	ingresses := make([]Ingress, 0, len(list.Items))
	for index := range list.Items {
		ingresses = append(ingresses, ingressDTO(&list.Items[index]))
	}
	return ingresses, nil
}

// ListHorizontalPodAutoscalers returns autoscaling/v2 HPAs in a namespace.
func (client *Client) ListHorizontalPodAutoscalers(ctx context.Context, namespace string) ([]HorizontalPodAutoscaler, error) {
	list, err := client.kubernetesInterface.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list hpas: %w", err)
	}
	hpas := make([]HorizontalPodAutoscaler, 0, len(list.Items))
	for index := range list.Items {
		hpas = append(hpas, horizontalPodAutoscalerDTO(&list.Items[index]))
	}
	return hpas, nil
}

// ListNamespaces returns every namespace in the cluster. Read-only; namespace
// allowlists are applied at the enforcer, not here.
func (client *Client) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	list, err := client.kubernetesInterface.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list namespaces: %w", err)
	}
	namespaces := make([]Namespace, 0, len(list.Items))
	for index := range list.Items {
		namespaces = append(namespaces, namespaceDTO(&list.Items[index]))
	}
	return namespaces, nil
}

// ListNodes returns nodes with zone, instance type, allocatable pod capacity,
// CPU/memory capacity, readiness, and live CPU/memory usage from metrics-
// server. Node addresses and arbitrary labels are stripped. If metrics-server
// is unreachable the usage fields stay at zero — the bars render empty rather
// than the whole list failing.
func (client *Client) ListNodes(ctx context.Context) ([]Node, error) {
	list, err := client.kubernetesInterface.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list nodes: %w", err)
	}
	usage := client.nodeMetricsByName(ctx)
	nodes := make([]Node, 0, len(list.Items))
	for index := range list.Items {
		nodes = append(nodes, nodeDTO(&list.Items[index], usage[list.Items[index].Name]))
	}
	return nodes, nil
}

// nodeUsage is the bit of metrics-server data the DTO consumes — already
// resolved against allocatable so the caller doesn't have to repeat the
// division. Both fields are 0..1 fractions (clamped on the way out).
type nodeUsage struct {
	cpu    float64
	memory float64
}

// podUsage carries a pod's summed container usage in raw units (cores +
// bytes). The fraction-of-node is computed later, once we know the host.
type podUsage struct {
	cpu    float64
	memory float64
}

// nodeCapacity holds the allocatable Quantities the pod DTO divides into to
// arrive at the 0..1 usage fraction.
type nodeCapacity struct {
	cpu    resource.Quantity
	memory resource.Quantity
}

func (client *Client) podMetricsByName(ctx context.Context, namespace string) map[string]podUsage {
	if client.metricsInterface == nil {
		return nil
	}
	list, err := client.metricsInterface.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	usage := make(map[string]podUsage, len(list.Items))
	for index := range list.Items {
		podMetrics := &list.Items[index]
		var cpu, memory float64
		for _, container := range podMetrics.Containers {
			cpuQuantity := container.Usage[corev1.ResourceCPU]
			memoryQuantity := container.Usage[corev1.ResourceMemory]
			cpu += cpuQuantity.AsApproximateFloat64()
			memory += memoryQuantity.AsApproximateFloat64()
		}
		usage[podMetrics.Name] = podUsage{cpu: cpu, memory: memory}
	}
	return usage
}

// nodeAllocatableByName fetches every node's allocatable CPU/memory so pod
// DTOs can compute their host-relative utilization. We tolerate failure here
// the same way we tolerate missing metrics: pod bars stay at zero rather than
// sinking the read.
func (client *Client) nodeAllocatableByName(ctx context.Context) map[string]nodeCapacity {
	list, err := client.kubernetesInterface.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	out := make(map[string]nodeCapacity, len(list.Items))
	for index := range list.Items {
		node := &list.Items[index]
		out[node.Name] = nodeCapacity{
			cpu:    node.Status.Allocatable[corev1.ResourceCPU],
			memory: node.Status.Allocatable[corev1.ResourceMemory],
		}
	}
	return out
}

func (client *Client) nodeMetricsByName(ctx context.Context) map[string]nodeUsage {
	if client.metricsInterface == nil {
		return nil
	}
	list, err := client.metricsInterface.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		// metrics-server not installed, RBAC missing, or transient — return
		// nothing; ListNodes already handles a zero map.
		return nil
	}
	usage := make(map[string]nodeUsage, len(list.Items))
	for index := range list.Items {
		nodeMetrics := &list.Items[index]
		cpu := nodeMetrics.Usage[corev1.ResourceCPU]
		memory := nodeMetrics.Usage[corev1.ResourceMemory]
		usage[nodeMetrics.Name] = nodeUsage{
			cpu:    cpu.AsApproximateFloat64(),
			memory: memory.AsApproximateFloat64(),
		}
	}
	return usage
}

// ClusterInfo returns the cluster's identity plus a health probe result.
// Name/region come from configuration; healthy reflects whether the apiserver
// answered a /livez call within clusterHealthProbeTimeout (or before the
// caller's ctx fires, whichever is sooner).
func (client *Client) ClusterInfo(ctx context.Context, name, region string) (ClusterInfo, error) {
	probeCtx, cancel := context.WithTimeout(ctx, clusterHealthProbeTimeout)
	defer cancel()
	err := client.health.probe(probeCtx)
	return ClusterInfo{
		Name:    name,
		Region:  region,
		Healthy: err == nil,
	}, nil
}

// ClusterHealth runs the same /livez probe as ClusterInfo but returns just the
// health verdict. Lets the UI poll health on its own cadence without paying
// for an identity round-trip every tick.
func (client *Client) ClusterHealth(ctx context.Context) (ClusterHealth, error) {
	probeCtx, cancel := context.WithTimeout(ctx, clusterHealthProbeTimeout)
	defer cancel()
	err := client.health.probe(probeCtx)
	return ClusterHealth{Healthy: err == nil}, nil
}

// ListReplicaSets returns ReplicaSets in a namespace with revision metadata.
func (client *Client) ListReplicaSets(ctx context.Context, namespace string) ([]ReplicaSet, error) {
	list, err := client.kubernetesInterface.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list replicasets: %w", err)
	}
	replicaSets := make([]ReplicaSet, 0, len(list.Items))
	for index := range list.Items {
		replicaSets = append(replicaSets, replicaSetDTO(&list.Items[index]))
	}
	return replicaSets, nil
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
	specContainers := deployment.Spec.Template.Spec.Containers
	containers := make([]DeploymentContainer, 0, len(specContainers))
	for index := range specContainers {
		containers = append(containers, DeploymentContainer{
			Name:  specContainers[index].Name,
			Image: specContainers[index].Image,
		})
	}
	return Deployment{
		Name:              deployment.Name,
		Namespace:         deployment.Namespace,
		Replicas:          replicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		UpdatedReplicas:   deployment.Status.UpdatedReplicas,
		Paused:            paused,
		Containers:        containers,
	}
}

func podDTO(pod *corev1.Pod, usage podUsage, host nodeCapacity) Pod {
	var restarts int32
	for _, status := range pod.Status.ContainerStatuses {
		restarts += status.RestartCount
	}
	ceiling := podResourceCeiling(pod, host)
	return Pod{
		Name:         pod.Name,
		Namespace:    pod.Namespace,
		Phase:        string(pod.Status.Phase),
		Labels:       pod.Labels,
		NodeName:     pod.Spec.NodeName,
		RestartCount: restarts,
		CreatedAt:    pod.CreationTimestamp.Time,
		CPUUsage:     fractionOf(usage.cpu, ceiling.cpu),
		MemoryUsage:  fractionOf(usage.memory, ceiling.memory),
	}
}

// podResourceCeiling resolves the denominator for a pod's CPU/memory usage:
// prefer the sum of container limits (the hard ceiling), fall back to the sum
// of requests (the soft ceiling — "what we asked for"), and finally to the
// host node's allocatable so unbounded pods still get a meaningful bar.
func podResourceCeiling(pod *corev1.Pod, hostFallback nodeCapacity) nodeCapacity {
	var cpuLimit, cpuRequest, memLimit, memRequest resource.Quantity
	for _, container := range pod.Spec.Containers {
		if value, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
			cpuLimit.Add(value)
		}
		if value, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
			cpuRequest.Add(value)
		}
		if value, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
			memLimit.Add(value)
		}
		if value, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
			memRequest.Add(value)
		}
	}
	return nodeCapacity{
		cpu:    pickCeiling(cpuLimit, cpuRequest, hostFallback.cpu),
		memory: pickCeiling(memLimit, memRequest, hostFallback.memory),
	}
}

func pickCeiling(limit, request, fallback resource.Quantity) resource.Quantity {
	if !limit.IsZero() {
		return limit
	}
	if !request.IsZero() {
		return request
	}
	return fallback
}

// nodeDTO projects a corev1.Node down to the DTO shape — well-known topology
// labels, allocatable pod count, raw CPU/memory capacity, Ready condition,
// and (when metrics-server is available) live CPU/memory utilization as a
// 0..1 fraction of allocatable.
func nodeDTO(node *corev1.Node, usage nodeUsage) Node {
	labels := node.Labels
	allocatable := node.Status.Allocatable
	podQuantity := allocatable[corev1.ResourcePods]
	podCapacity, _ := podQuantity.AsInt64()
	cpuCapacity := allocatable[corev1.ResourceCPU]
	memoryCapacity := allocatable[corev1.ResourceMemory]
	ready := false
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			ready = condition.Status == corev1.ConditionTrue
			break
		}
	}
	zone := labels[corev1.LabelTopologyZone]
	if zone == "" {
		zone = labels["failure-domain.beta.kubernetes.io/zone"]
	}
	instanceType := labels[corev1.LabelInstanceTypeStable]
	if instanceType == "" {
		instanceType = labels[corev1.LabelInstanceType]
	}
	return Node{
		Name:           node.Name,
		Zone:           zone,
		InstanceType:   instanceType,
		PodCapacity:    podCapacity,
		CPUCapacity:    cpuCapacity.String(),
		MemoryCapacity: memoryCapacity.String(),
		CPUUsage:       fractionOf(usage.cpu, cpuCapacity),
		MemoryUsage:    fractionOf(usage.memory, memoryCapacity),
		Ready:          ready,
	}
}

// fractionOf returns used / ceiling clamped to [0, 1]. Returns zero when the
// ceiling is missing so the bar renders empty instead of NaN. Used by both
// the node DTO (ceiling = allocatable) and the pod DTO (ceiling = limits →
// requests → host allocatable).
func fractionOf(used float64, ceiling resource.Quantity) float64 {
	denominator := ceiling.AsApproximateFloat64()
	if denominator <= 0 || used <= 0 {
		return 0
	}
	fraction := used / denominator
	if fraction > 1 {
		return 1
	}
	return fraction
}

func serviceDTO(service *corev1.Service) Service {
	ports := make([]ServicePort, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		ports = append(ports, ServicePort{
			Name:       port.Name,
			Port:       port.Port,
			TargetPort: port.TargetPort.String(),
			Protocol:   string(port.Protocol),
			NodePort:   port.NodePort,
		})
	}
	return Service{
		Name:      service.Name,
		Namespace: service.Namespace,
		Type:      string(service.Spec.Type),
		ClusterIP: service.Spec.ClusterIP,
		Ports:     ports,
	}
}

func ingressDTO(ingress *networkingv1.Ingress) Ingress {
	hosts := make([]string, 0, len(ingress.Spec.Rules))
	for _, rule := range ingress.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	className := ""
	if ingress.Spec.IngressClassName != nil {
		className = *ingress.Spec.IngressClassName
	}
	return Ingress{
		Name:      ingress.Name,
		Namespace: ingress.Namespace,
		Class:     className,
		Hosts:     hosts,
	}
}

func horizontalPodAutoscalerDTO(hpa *autoscalingv2.HorizontalPodAutoscaler) HorizontalPodAutoscaler {
	var minReplicas int32
	if hpa.Spec.MinReplicas != nil {
		minReplicas = *hpa.Spec.MinReplicas
	}
	target := ""
	if hpa.Spec.ScaleTargetRef.Kind != "" || hpa.Spec.ScaleTargetRef.Name != "" {
		target = hpa.Spec.ScaleTargetRef.Kind + "/" + hpa.Spec.ScaleTargetRef.Name
	}
	return HorizontalPodAutoscaler{
		Name:            hpa.Name,
		Namespace:       hpa.Namespace,
		MinReplicas:     minReplicas,
		MaxReplicas:     hpa.Spec.MaxReplicas,
		CurrentReplicas: hpa.Status.CurrentReplicas,
		TargetRef:       target,
	}
}

func namespaceDTO(namespace *corev1.Namespace) Namespace {
	return Namespace{Name: namespace.Name, Phase: string(namespace.Status.Phase)}
}

func replicaSetDTO(replicaSet *appsv1.ReplicaSet) ReplicaSet {
	var replicas int32
	if replicaSet.Spec.Replicas != nil {
		replicas = *replicaSet.Spec.Replicas
	}
	owner := ""
	for _, ownerRef := range replicaSet.OwnerReferences {
		if ownerRef.Kind == "Deployment" {
			owner = ownerRef.Name
			break
		}
	}
	return ReplicaSet{
		Name:              replicaSet.Name,
		Namespace:         replicaSet.Namespace,
		Replicas:          replicas,
		AvailableReplicas: replicaSet.Status.AvailableReplicas,
		Revision:          parseRevision(replicaSet.Annotations[revisionAnnotation]),
		Owner:             owner,
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
