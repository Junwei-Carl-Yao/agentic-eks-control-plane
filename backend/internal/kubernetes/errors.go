package kubernetes

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrNotFound is the sentinel returned when a resource lookup misses.
// Wraps both our internal misses and apierrors.IsNotFound from the K8s client,
// so route layers can map cleanly to HTTP 404 with one IsNotFound check.
var ErrNotFound = errors.New("kubernetes: resource not found")

// IsNotFound reports whether err indicates a missing resource.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	return apierrors.IsNotFound(err)
}
