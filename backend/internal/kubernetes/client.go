package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	"eks-control-plane/backend/internal/config"
)

// inClusterTokenPath is the standard path where the kubelet mounts the
// ServiceAccount token in a Pod. Its presence is the trigger for in-cluster
// config selection.
const inClusterTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// Client wraps a typed Kubernetes clientset, an optional metrics clientset
// (metrics.k8s.io/v1beta1 — nil if metrics-server is not installed), plus the
// log source we use for TailLogs and the health probe we use for ClusterInfo.
// Both seams exist so tests can inject in-memory fakes; the K8s fake
// clientset's Discovery().RESTClient() returns nil, which would panic the
// production probe path.
type Client struct {
	kubernetesInterface kubernetes.Interface
	metricsInterface    metricsclient.Interface
	logs                logSource
	health              healthProbe
}

// Interface returns the underlying clientset. Exposed mainly for tests and for
// callers that need direct access to typed APIs we have not yet wrapped.
func (client *Client) Interface() kubernetes.Interface { return client.kubernetesInterface }

// NewClient builds a Client from the standard config sources. In-cluster config
// is preferred when the ServiceAccount token is present (we are running inside
// a Pod). Otherwise we fall back to KUBECONFIG. If neither is available we
// return an error rather than silently constructing an unusable client. The
// metrics clientset is built off the same rest.Config; reads against it
// degrade gracefully (zero usage) if metrics-server isn't installed.
func NewClient(settings config.Settings) (*Client, error) {
	restConfig, err := buildRESTConfig(settings)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: build clientset: %w", err)
	}
	metricsSet, err := metricsclient.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: build metrics clientset: %w", err)
	}
	return &Client{
		kubernetesInterface: clientset,
		metricsInterface:    metricsSet,
		logs:                &kubeLogSource{kubernetesInterface: clientset},
		health:              &discoveryHealthProbe{discovery: clientset.Discovery()},
	}, nil
}

// healthProbe is the seam separating the production apiserver /livez probe
// from in-memory test fakes. ServerVersion() doesn't accept a context, so we
// hit /livez through the discovery REST client instead and let ctx cancel
// the in-flight request directly.
type healthProbe interface {
	probe(ctx context.Context) error
}

type discoveryHealthProbe struct {
	discovery discovery.DiscoveryInterface
}

func (probe *discoveryHealthProbe) probe(ctx context.Context) error {
	return probe.discovery.RESTClient().Get().AbsPath("/livez").Do(ctx).Error()
}

func buildRESTConfig(settings config.Settings) (*rest.Config, error) {
	if inClusterTokenPresent() {
		if restConfig, err := rest.InClusterConfig(); err == nil {
			return restConfig, nil
		}
	}
	if settings.Kubeconfig == "" {
		return nil, errors.New("kubernetes: no KUBECONFIG and not running in-cluster")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", settings.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: load kubeconfig: %w", err)
	}
	return restConfig, nil
}

func inClusterTokenPresent() bool {
	_, err := os.Stat(inClusterTokenPath)
	return err == nil
}
