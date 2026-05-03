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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

// fakeOption seeds objects into the fake clientset before it's wrapped in a
// Client. Tests assemble a fake cluster via newFakeClient(t, opt1, opt2, ...).
type fakeOption func(*fakeBuilder)

type fakeBuilder struct {
	objects []runtime.Object
	logs    map[logKey]string
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
	return &Client{
		kubernetesInterface: clientset,
		logs:                &memLogSource{logs: builder.logs},
	}
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

// withContainerEnv seeds a deployment whose named container has the given env
// vars (declared inline, not envFrom).
func withContainerEnv(namespace, name, container string, env map[string]string) fakeOption {
	return func(builder *fakeBuilder) {
		deployment := makeDeployment(namespace, name, 1)
		envVars := make([]corev1.EnvVar, 0, len(env))
		for key, value := range env {
			envVars = append(envVars, corev1.EnvVar{Name: key, Value: value})
		}
		deployment.Spec.Template.Spec.Containers = []corev1.Container{{Name: container, Env: envVars}}
		builder.objects = append(builder.objects, deployment)
	}
}

// withEnvFrom seeds a deployment whose named container references a configmap
// via envFrom — used to assert UpdateEnv leaves it alone.
func withEnvFrom(namespace, name, container, configMap string) fakeOption {
	return func(builder *fakeBuilder) {
		deployment := makeDeployment(namespace, name, 1)
		deployment.Spec.Template.Spec.Containers = []corev1.Container{{
			Name: container,
			EnvFrom: []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMap},
				},
			}},
		}}
		builder.objects = append(builder.objects, deployment)
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
