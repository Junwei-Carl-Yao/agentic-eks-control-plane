package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"eks-control-plane/backend/internal/config"
	"eks-control-plane/backend/internal/guardrails"
	"eks-control-plane/backend/internal/kubernetes"
)

// stubReads is a ClusterReader test double. It's used in two modes:
//   - by-value (`stubReads{deployments: ...}`) when the test only needs to
//     supply data and doesn't care about side effects;
//   - by-pointer (`&stubReads{}`) when the test reads back recorded fields
//     (lastPodSelector, lastLogPod, ...).
//
// newTestHandlerWithReads accepts both forms.
type stubReads struct {
	deployments   []Deployment
	deployment    *Deployment
	events        []Event
	services      []Service
	ingresses     []Ingress
	hpas          []HorizontalPodAutoscaler
	namespaces    []Namespace
	nodes         []Node
	replicaSets   []ReplicaSet
	clusterInfo   *ClusterInfo
	clusterHealth *ClusterHealth
	notFound      bool

	lastPodSelector  string
	lastLogPod       string
	lastLogContainer string
	lastLogLines     int64
	lastInfoName     string
	lastInfoRegion   string
	lastHealthCalls  int
}

func (stub *stubReads) ListDeployments(_ context.Context, _ string) ([]Deployment, error) {
	return stub.deployments, nil
}

func (stub *stubReads) GetDeployment(_ context.Context, namespace, name string) (Deployment, error) {
	if stub.notFound {
		return Deployment{}, fmt.Errorf("%w: %s/%s", kubernetes.ErrNotFound, namespace, name)
	}
	if stub.deployment != nil {
		return *stub.deployment, nil
	}
	return Deployment{}, nil
}

func (stub *stubReads) ListPods(_ context.Context, _ string, selector string) ([]Pod, error) {
	stub.lastPodSelector = selector
	return nil, nil
}

func (stub *stubReads) ListEvents(_ context.Context, _ string) ([]Event, error) {
	return stub.events, nil
}

func (stub *stubReads) TailLogs(_ context.Context, _, pod, container string, lines int64) (string, error) {
	stub.lastLogPod = pod
	stub.lastLogContainer = container
	stub.lastLogLines = lines
	return "", nil
}

func (stub *stubReads) ListServices(_ context.Context, _ string) ([]Service, error) {
	return stub.services, nil
}

func (stub *stubReads) ListIngresses(_ context.Context, _ string) ([]Ingress, error) {
	return stub.ingresses, nil
}

func (stub *stubReads) ListHorizontalPodAutoscalers(_ context.Context, _ string) ([]HorizontalPodAutoscaler, error) {
	return stub.hpas, nil
}

func (stub *stubReads) ListNamespaces(_ context.Context) ([]Namespace, error) {
	return stub.namespaces, nil
}

func (stub *stubReads) ListNodes(_ context.Context) ([]Node, error) {
	return stub.nodes, nil
}

func (stub *stubReads) ListReplicaSets(_ context.Context, _ string) ([]ReplicaSet, error) {
	return stub.replicaSets, nil
}

func (stub *stubReads) ClusterInfo(_ context.Context, name, region string) (ClusterInfo, error) {
	stub.lastInfoName = name
	stub.lastInfoRegion = region
	if stub.clusterInfo == nil {
		return ClusterInfo{Name: name, Region: region, Healthy: true}, nil
	}
	return *stub.clusterInfo, nil
}

func (stub *stubReads) ClusterHealth(_ context.Context) (ClusterHealth, error) {
	stub.lastHealthCalls++
	if stub.clusterHealth == nil {
		return ClusterHealth{Healthy: true}, nil
	}
	return *stub.clusterHealth, nil
}

// stubOps is the Operations test double. All call recording is on a pointer
// receiver, so tests must use `&stubOps{}` to read back fields.
type stubOps struct {
	scaleCalls           int
	lastScaleReplicas    int32
	lastRestart          string
	paused               bool
	resumed              bool
	lastRollbackRevision int64
	notFound             bool
}

func (stub *stubOps) maybeNotFound(namespace, name string) error {
	if stub.notFound {
		return fmt.Errorf("%w: %s/%s", kubernetes.ErrNotFound, namespace, name)
	}
	return nil
}

func (stub *stubOps) Scale(_ context.Context, namespace, name string, replicas int32) error {
	if err := stub.maybeNotFound(namespace, name); err != nil {
		return err
	}
	stub.scaleCalls++
	stub.lastScaleReplicas = replicas
	return nil
}

func (stub *stubOps) RolloutRestart(_ context.Context, namespace, name string) error {
	if err := stub.maybeNotFound(namespace, name); err != nil {
		return err
	}
	stub.lastRestart = namespace + "/" + name
	return nil
}

func (stub *stubOps) PauseRollout(_ context.Context, namespace, name string) error {
	if err := stub.maybeNotFound(namespace, name); err != nil {
		return err
	}
	stub.paused = true
	return nil
}

func (stub *stubOps) ResumeRollout(_ context.Context, namespace, name string) error {
	if err := stub.maybeNotFound(namespace, name); err != nil {
		return err
	}
	stub.resumed = true
	return nil
}

func (stub *stubOps) Rollback(_ context.Context, namespace, name string, revision int64) error {
	if err := stub.maybeNotFound(namespace, name); err != nil {
		return err
	}
	stub.lastRollbackRevision = revision
	return nil
}

// --- handler builders ---

// testPolicy is the policy server tests assume: namespace "app". It's
// deliberately different from DefaultPolicy — server tests scope their world
// locally instead of mutating exported package state.
func testPolicy() guardrails.Policy {
	return guardrails.Policy{
		AllowedNamespaces: []string{"app"},
	}
}

// permissiveEnforcer returns an Enforcer wired to testPolicy. Use this when a
// test is asserting plumbing, not policy.
func permissiveEnforcer() *guardrails.Enforcer {
	return guardrails.New(testPolicy(), slog.Default())
}

func newTestHandlerWithReads(reader any) http.Handler {
	return New(config.Settings{}, Deps{Reader: toReader(reader), Enforcer: permissiveEnforcer()})
}

func newTestHandlerWithReadsAndEnforcer(reader any, enforcer *guardrails.Enforcer) http.Handler {
	return New(config.Settings{}, Deps{Reader: toReader(reader), Enforcer: enforcer})
}

func newTestHandlerWithOps(ops *stubOps) http.Handler {
	return New(config.Settings{}, Deps{Ops: ops, Enforcer: permissiveEnforcer()})
}

func newTestHandlerWithOpsAndEnforcer(ops *stubOps, enforcer *guardrails.Enforcer) http.Handler {
	return New(config.Settings{}, Deps{Ops: ops, Enforcer: enforcer})
}

// toReader normalises stubReads / *stubReads to a ClusterReader. Value-form
// inputs are taken by address into a fresh copy; the test isn't expected to
// read state back in that case.
func toReader(reader any) ClusterReader {
	switch typed := reader.(type) {
	case *stubReads:
		return typed
	case stubReads:
		return &typed
	default:
		panic(fmt.Sprintf("toReader: unsupported type %T", reader))
	}
}
