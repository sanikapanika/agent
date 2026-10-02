package monitor

import (
	"errors"
	"fmt"
	"strings"
)

func init() {
	RegisterCheckType(CheckType{
		Type:     TypeKubernetes,
		Label:    "Kubernetes",
		Summary:  "Deployment, StatefulSet, DaemonSet or Service ready",
		Order:    50,
		Target:   TargetSpec{Label: "Workload or Service", Placeholder: "default/deployment/api"},
		Requires: "kubernetes",
		Normalize: func(c *Check) error {
			_, _, _, err := ParseKubernetesTarget(c.Target)
			return err
		},
	})
}

// ParseKubernetesTarget splits "namespace/kind/name". A workload is up when
// all its replicas are ready; a Service when it has a ready endpoint.
func ParseKubernetesTarget(target string) (namespace, kind, name string, err error) {
	parts := strings.Split(target, "/")
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return "", "", "", errors.New("target must be namespace/kind/name, e.g. default/deployment/api")
	}
	kind = strings.ToLower(parts[1])
	switch kind {
	case "deployment", "statefulset", "daemonset", "service":
	default:
		return "", "", "", fmt.Errorf("unsupported kind %q (use deployment, statefulset, daemonset or service)", parts[1])
	}
	return parts[0], kind, parts[2], nil
}
