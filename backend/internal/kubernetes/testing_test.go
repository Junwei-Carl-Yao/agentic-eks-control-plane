package kubernetes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

// fakeOption seeds objects into the fake clientset before it's wrapped in a
// Client. Tests assemble a fake cluster via newFakeClient(t, opt1, opt2, ...).
type fakeOption func(*fakeBuilder)

type fakeBuilder struct {
	objects     []runtime.Object
	nodeMetrics []metricsv1beta1.NodeMetrics
	podMetrics  []metricsv1beta1.PodMetrics
	logs        map[logKey]string
	healthErr   error
}

type logKey struct{ namespace, pod, container string }

// newFakeClient builds a Client backed by the K8s fake clientset and an
// in-memory log store so tests don't need to wire the real GetLogs subresource.
func newFakeClient(t *testing.T, options ...fakeOption) *Client {
	t.Helper()
	builder := &fakeBuilder{logs: map[logKey]string{}}
	for _, option := range options {
		option(builder)
	}
	clientset := fake.NewSimpleClientset(builder.objects...)
	// metricsfake's tracker doesn't surface NodeMetricsList on List(), so we
	// install a reactor that hands back whatever the test seeded. Empty list
	// reactor stands in for "metrics-server installed but no metrics yet".
	metricsClientset := metricsfake.NewSimpleClientset()
	seededNodeMetrics := append([]metricsv1beta1.NodeMetrics(nil), builder.nodeMetrics...)
	metricsClientset.PrependReactor("list", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.NodeMetricsList{Items: seededNodeMetrics}, nil
	})
	seededPodMetrics := append([]metricsv1beta1.PodMetrics(nil), builder.podMetrics...)
	metricsClientset.PrependReactor("list", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		namespace := action.GetNamespace()
		if namespace == "" {
			return true, &metricsv1beta1.PodMetricsList{Items: seededPodMetrics}, nil
		}
		filtered := make([]metricsv1beta1.PodMetrics, 0, len(seededPodMetrics))
		for _, podMetrics := range seededPodMetrics {
			if podMetrics.Namespace == namespace {
				filtered = append(filtered, podMetrics)
			}
		}
		return true, &metricsv1beta1.PodMetricsList{Items: filtered}, nil
	})
	return &Client{
		kubernetesInterface: clientset,
		metricsInterface:    metricsClientset,
		logs:                &memLogSource{logs: builder.logs},
		health:              &memHealthProbe{err: builder.healthErr},
	}
}

// withUnhealthyProbe makes the in-memory health probe return err, simulating
// an unreachable apiserver. Tests that don't supply this option get a healthy
// probe by default.
func withUnhealthyProbe(err error) fakeOption {
	return func(builder *fakeBuilder) {
		builder.healthErr = err
	}
}

// memHealthProbe is the in-memory healthProbe used by newFakeClient. The fake
// clientset's Discovery().RESTClient() is nil, so the production probe path
// would panic — this stand-in returns a configurable error instead.
type memHealthProbe struct {
	err error
}

func (probe *memHealthProbe) probe(_ context.Context) error {
	return probe.err
}

// withDeployments seeds N deployments named after the trailing args.
func withDeployments(namespace string, names ...string) fakeOption {
	return func(builder *fakeBuilder) {
		for _, name := range names {
			builder.objects = append(builder.objects, makeDeployment(namespace, name, 1))
		}
	}
}

// withDeploymentReplicas seeds a deployment with a specific replica count.
func withDeploymentReplicas(namespace, name string, replicas int32) fakeOption {
	return func(builder *fakeBuilder) {
		builder.objects = append(builder.objects, makeDeployment(namespace, name, replicas))
	}
}

// withPausedDeployment seeds a paused deployment.
func withPausedDeployment(namespace, name string) fakeOption {
	return func(builder *fakeBuilder) {
		deployment := makeDeployment(namespace, name, 1)
		deployment.Spec.Paused = true
		builder.objects = append(builder.objects, deployment)
	}
}

// withDeploymentContainers seeds a deployment whose pod template carries the
// given (name, image) pairs as primary containers in declaration order.
// imagePairs must contain an even number of strings: name1, image1, name2, image2, ...
func withDeploymentContainers(namespace, name string, imagePairs ...string) fakeOption {
	return func(builder *fakeBuilder) {
		if len(imagePairs)%2 != 0 {
			panic("withDeploymentContainers: imagePairs must be (name,image)*")
		}
		containers := make([]corev1.Container, 0, len(imagePairs)/2)
		for index := 0; index < len(imagePairs); index += 2 {
			containers = append(containers, corev1.Container{
				Name:  imagePairs[index],
				Image: imagePairs[index+1],
			})
		}
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: containers},
				},
			},
		}
		builder.objects = append(builder.objects, deployment)
	}
}

// withDeploymentContainersAndInit seeds a deployment with both primary
// containers and init containers, so tests can prove init containers do NOT
// leak into the DTO's Containers slice.
func withDeploymentContainersAndInit(namespace, name string, primary, init []corev1.Container) fakeOption {
	return func(builder *fakeBuilder) {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers:     primary,
						InitContainers: init,
					},
				},
			},
		}
		builder.objects = append(builder.objects, deployment)
	}
}

// withRevisionHistory seeds a deployment plus one ReplicaSet per revision. The
// deployment's current revision is the largest entry in revisions.
func withRevisionHistory(namespace, name string, revisions []int64) fakeOption {
	return func(builder *fakeBuilder) {
		var current int64
		for _, revision := range revisions {
			if revision > current {
				current = revision
			}
		}
		deployment := makeDeployment(namespace, name, 1)
		deployment.UID = types.UID(fmt.Sprintf("%s-uid", name))
		deployment.Annotations = map[string]string{
			revisionAnnotation: strconv.FormatInt(current, 10),
		}
		builder.objects = append(builder.objects, deployment)
		for _, revision := range revisions {
			replicaSet := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-rev%d", name, revision),
					Annotations: map[string]string{
						revisionAnnotation: strconv.FormatInt(revision, 10),
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       name,
						UID:        deployment.UID,
					}},
				},
				Spec: appsv1.ReplicaSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"revision": strconv.FormatInt(revision, 10),
							},
						},
					},
				},
			}
			builder.objects = append(builder.objects, replicaSet)
		}
	}
}

// revisionContainers describes one historical ReplicaSet for a deployment:
// the revision number and the container set frozen into that revision's pod
// template. Used by withRevisionHistoryAndContainers to seed enough state for
// the rollback image-resolution path without painting containers onto every
// historical ReplicaSet by hand.
type revisionContainers struct {
	revision   int64
	containers []corev1.Container
}

// withRevisionHistoryAndContainers seeds a deployment whose ReplicaSets each
// carry their own container set. Mirrors withRevisionHistory but lets the test
// pin the image-per-revision contents the rollback resolver inspects.
func withRevisionHistoryAndContainers(namespace, deploymentName string, history []revisionContainers) fakeOption {
	return func(builder *fakeBuilder) {
		var currentRevision int64
		for _, entry := range history {
			if entry.revision > currentRevision {
				currentRevision = entry.revision
			}
		}
		deployment := makeDeployment(namespace, deploymentName, 1)
		deployment.UID = types.UID(fmt.Sprintf("%s-uid", deploymentName))
		deployment.Annotations = map[string]string{
			revisionAnnotation: strconv.FormatInt(currentRevision, 10),
		}
		builder.objects = append(builder.objects, deployment)
		for _, entry := range history {
			replicaSet := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-rev%d", deploymentName, entry.revision),
					Annotations: map[string]string{
						revisionAnnotation: strconv.FormatInt(entry.revision, 10),
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       deploymentName,
						UID:        deployment.UID,
					}},
				},
				Spec: appsv1.ReplicaSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{Containers: entry.containers},
					},
				},
			}
			builder.objects = append(builder.objects, replicaSet)
		}
	}
}

// withService seeds a ClusterIP Service exposing one port.
func withService(namespace, name string, port int32) fakeOption {
	return func(builder *fakeBuilder) {
		builder.objects = append(builder.objects, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0.1",
				Ports:     []corev1.ServicePort{{Port: port, Protocol: corev1.ProtocolTCP}},
			},
		})
	}
}

// withIngress seeds a single-host Ingress.
func withIngress(namespace, name, host string) fakeOption {
	return func(builder *fakeBuilder) {
		builder.objects = append(builder.objects, &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec:       netv1.IngressSpec{Rules: []netv1.IngressRule{{Host: host}}},
		})
	}
}

// withHorizontalPodAutoscaler seeds an HPA targeting a Deployment.
func withHorizontalPodAutoscaler(namespace, name, target string, min, max int32) fakeOption {
	return func(builder *fakeBuilder) {
		minPtr := min
		builder.objects = append(builder.objects, &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: &minPtr,
				MaxReplicas: max,
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment",
					Name: target,
				},
			},
		})
	}
}

// withNamespace seeds a Namespace object.
func withNamespace(name string) fakeOption {
	return func(builder *fakeBuilder) {
		builder.objects = append(builder.objects, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		})
	}
}

// withNode seeds a bare Node — no labels, no allocatable, no Ready condition.
// Useful for exercising the zero-value branches of nodeDTO.
func withNode(name string) fakeOption {
	return func(builder *fakeBuilder) {
		builder.objects = append(builder.objects, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		})
	}
}

// withNodeDetailed seeds a Node populated with the topology labels, allocatable
// resources, and Ready condition that nodeDTO projects onto the wire.
func withNodeDetailed(name, zone, instanceType string, podCapacity int64, cpu, memory string, ready bool) fakeOption {
	return func(builder *fakeBuilder) {
		condition := corev1.ConditionFalse
		if ready {
			condition = corev1.ConditionTrue
		}
		builder.objects = append(builder.objects, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					corev1.LabelTopologyZone:       zone,
					corev1.LabelInstanceTypeStable: instanceType,
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourcePods:   *resource.NewQuantity(podCapacity, resource.DecimalSI),
					corev1.ResourceCPU:    resource.MustParse(cpu),
					corev1.ResourceMemory: resource.MustParse(memory),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: condition},
				},
			},
		})
	}
}

// withPod seeds a pod with optional labels.
func withPod(namespace, name string, labels map[string]string) fakeOption {
	return func(builder *fakeBuilder) {
		builder.objects = append(builder.objects, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
				Labels:    labels,
			},
		})
	}
}

// withNodeMetrics seeds a metrics-server NodeMetrics record for a node. The
// cpu/memory args are raw Quantity strings, e.g. "1500m", "8Gi" — the same
// shape `kubectl top node` reports.
func withNodeMetrics(name, cpu, memory string) fakeOption {
	return func(builder *fakeBuilder) {
		builder.nodeMetrics = append(builder.nodeMetrics, metricsv1beta1.NodeMetrics{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
			},
		})
	}
}

// withPodMetrics seeds a metrics-server PodMetrics record. The cpu/memory
// args are summed across a single container the same way the production
// shape would after the pod's containers report.
func withPodMetrics(namespace, name, cpu, memory string) fakeOption {
	return func(builder *fakeBuilder) {
		builder.podMetrics = append(builder.podMetrics, metricsv1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Containers: []metricsv1beta1.ContainerMetrics{
				{
					Name: "app",
					Usage: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(cpu),
						corev1.ResourceMemory: resource.MustParse(memory),
					},
				},
			},
		})
	}
}

// withScheduledPodLimits seeds a pod placed on a node with explicit CPU/mem
// limits and requests on its single container. Empty strings skip that field
// so callers can exercise the limits-only, requests-only, or unbounded paths
// in podResourceCeiling. Always uses Running phase + zero restarts.
func withScheduledPodLimits(namespace, name, nodeName string, createdAt time.Time, cpuLimit, memoryLimit, cpuRequest, memoryRequest string) fakeOption {
	return func(builder *fakeBuilder) {
		container := corev1.Container{Name: "app"}
		container.Resources = corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{},
			Requests: corev1.ResourceList{},
		}
		if cpuLimit != "" {
			container.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(cpuLimit)
		}
		if memoryLimit != "" {
			container.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(memoryLimit)
		}
		if cpuRequest != "" {
			container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpuRequest)
		}
		if memoryRequest != "" {
			container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(memoryRequest)
		}
		builder.objects = append(builder.objects, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:         namespace,
				Name:              name,
				CreationTimestamp: metav1.NewTime(createdAt),
			},
			Spec: corev1.PodSpec{
				NodeName:   nodeName,
				Containers: []corev1.Container{container},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", RestartCount: 0},
				},
			},
		})
	}
}

// withScheduledPod seeds a pod placed on a node, with a creation timestamp and
// a single container that has restarted `restarts` times.
func withScheduledPod(namespace, name, nodeName string, restarts int32, createdAt time.Time) fakeOption {
	return func(builder *fakeBuilder) {
		builder.objects = append(builder.objects, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:         namespace,
				Name:              name,
				CreationTimestamp: metav1.NewTime(createdAt),
			},
			Spec: corev1.PodSpec{NodeName: nodeName},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", RestartCount: restarts},
				},
			},
		})
	}
}

// withEventsAtMinutes seeds one Event per `minuteOffsets` value, with LastTimestamp set
// to `now - X minutes`. Used to verify newest-first ordering.
func withEventsAtMinutes(namespace string, minuteOffsets ...int) fakeOption {
	return func(builder *fakeBuilder) {
		now := time.Now().UTC()
		for index, minutesAgo := range minuteOffsets {
			timestamp := metav1.NewTime(now.Add(-time.Duration(minutesAgo) * time.Minute))
			builder.objects = append(builder.objects, &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      fmt.Sprintf("evt-%d", index),
				},
				LastTimestamp: timestamp,
				Reason:        fmt.Sprintf("R%d", index),
			})
		}
	}
}

// withPodLogs records a canned log body for a (ns, pod, container) tuple.
func withPodLogs(namespace, pod, container, body string) fakeOption {
	return func(builder *fakeBuilder) {
		builder.logs[logKey{namespace, pod, container}] = body
	}
}

// memLogSource implements logSource against an in-memory map. Returns
// IsNotFound when the tuple was never seeded.
type memLogSource struct {
	mutex sync.Mutex
	logs  map[logKey]string
}

func (source *memLogSource) Tail(_ context.Context, namespace, pod, container string, lines int64) (string, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	body, ok := source.logs[logKey{namespace, pod, container}]
	if !ok {
		return "", fmt.Errorf("%w: pod %s/%s container %s logs", ErrNotFound, namespace, pod, container)
	}
	return tailLines(body, lines), nil
}

func makeDeployment(namespace, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: "nginx"}}},
			},
		},
	}
}

// testKubeconfigPath writes a minimal valid kubeconfig file and returns its
// path. Pointing KUBECONFIG at this is enough for clientcmd.BuildConfigFromFlags
// to construct a *rest.Config without actually connecting.
func testKubeconfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	contents := `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: test-token
`
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}
