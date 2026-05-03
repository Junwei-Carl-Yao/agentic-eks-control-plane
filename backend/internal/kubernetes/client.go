package kubernetes

import (
	"errors"
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"eks-control-plane/backend/internal/config"
)

// inClusterTokenPath is the standard path where the kubelet mounts the
// ServiceAccount token in a Pod. Its presence is the trigger for in-cluster
// config selection.
const inClusterTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// Client wraps a typed Kubernetes clientset plus the log source we use for
// TailLogs. The log source is a separate seam so tests can inject in-memory
// log fixtures without standing up a fake API server's logs subresource.
type Client struct {
	kubernetesInterface kubernetes.Interface
	logs                logSource
}

// Interface returns the underlying clientset. Exposed mainly for tests and for
// callers that need direct access to typed APIs we have not yet wrapped.
func (client *Client) Interface() kubernetes.Interface { return client.kubernetesInterface }

// NewClient builds a Client from the standard config sources. In-cluster config
// is preferred when the ServiceAccount token is present (we are running inside
// a Pod). Otherwise we fall back to KUBECONFIG. If neither is available we
// return an error rather than silently constructing an unusable client.
func NewClient(settings config.Settings) (*Client, error) {
	restConfig, err := buildRESTConfig(settings)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: build clientset: %w", err)
	}
	return &Client{kubernetesInterface: clientset, logs: &kubeLogSource{kubernetesInterface: clientset}}, nil
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
