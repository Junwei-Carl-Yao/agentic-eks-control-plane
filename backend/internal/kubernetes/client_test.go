// Phase 2.2 — Kubernetes client construction.
package kubernetes

import (
	"testing"

	"eks-control-plane/backend/internal/config"
)

// Scenario: KUBECONFIG path is set → NewClient builds an out-of-cluster client.
func TestNewClient_FromKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", testKubeconfigPath(t))
	kubeClient, err := NewClient(config.Load())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if kubeClient == nil || kubeClient.Interface() == nil {
		t.Fatal("NewClient returned nil client/interface")
	}
}

// Scenario: in-cluster service-account token is mounted at the standard path → NewClient
// uses InClusterConfig instead of KUBECONFIG. (Skipped unless the token file exists; this
// test asserts the *selection rule*, not connectivity.)
func TestNewClient_InClusterPreferredWhenAvailable(t *testing.T) {
	if !inClusterTokenPresent() {
		t.Skip("not running in-cluster; selection rule covered by integration env")
	}
	t.Setenv("KUBECONFIG", "/dev/null")
	if _, err := NewClient(config.Load()); err != nil {
		t.Fatalf("expected in-cluster config to win over invalid KUBECONFIG: %v", err)
	}
}

// Scenario: neither KUBECONFIG nor in-cluster config is available → NewClient returns
// an error rather than silently constructing an unusable client.
func TestNewClient_NoConfigReturnsError(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	if _, err := NewClient(config.Load()); err == nil {
		t.Fatal("expected error when no kubeconfig and not in-cluster")
	}
}
