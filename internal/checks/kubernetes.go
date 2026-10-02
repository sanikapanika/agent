package checks

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptimy/agent/internal/kube"
	"github.com/uptimy/agent/internal/monitor"
)

func init() {
	register(monitor.TypeKubernetes, func(ctx context.Context, c *Checker, m monitor.Check) Outcome {
		if c.kube == nil {
			return Outcome{Message: "not running inside Kubernetes (no service account found)"}
		}
		return checkWorkload(ctx, c.kube, m.Target)
	})
}

type workloadStatus struct {
	Spec struct {
		Replicas *int `json:"replicas"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas          int `json:"readyReplicas"`
		AvailableReplicas      int `json:"availableReplicas"`
		DesiredNumberScheduled int `json:"desiredNumberScheduled"`
		NumberReady            int `json:"numberReady"`
	} `json:"status"`
}

// checkWorkload reports whether a workload has all its replicas ready.
func checkWorkload(ctx context.Context, k *kube.Client, target string) Outcome {
	ns, kind, name, err := monitor.ParseKubernetesTarget(target)
	if err != nil {
		return Outcome{Message: err.Error()}
	}
	var w workloadStatus
	err = k.Get(ctx, fmt.Sprintf("/apis/apps/v1/namespaces/%s/%ss/%s", ns, kind, name), &w)
	var se *kube.StatusError
	switch {
	case kube.IsNotFound(err):
		return Outcome{Message: fmt.Sprintf("%s %s/%s not found", kind, ns, name)}
	case kube.IsForbidden(err):
		return Outcome{Message: "forbidden: the agent's service account can't read " + kind + "s (check RBAC)"}
	case errors.As(err, &se):
		return Outcome{Message: se.Error()}
	case err != nil:
		return Outcome{Message: trimErr(err)}
	}
	var ready, desired int
	switch kind {
	case "deployment":
		desired, ready = 1, w.Status.AvailableReplicas
		if w.Spec.Replicas != nil {
			desired = *w.Spec.Replicas
		}
	case "statefulset":
		desired, ready = 1, w.Status.ReadyReplicas
		if w.Spec.Replicas != nil {
			desired = *w.Spec.Replicas
		}
	case "daemonset":
		desired, ready = w.Status.DesiredNumberScheduled, w.Status.NumberReady
	}
	msg := fmt.Sprintf("%d/%d ready", ready, desired)
	if desired == 0 {
		return Outcome{OK: true, Message: "scaled to zero"}
	}
	return Outcome{OK: ready >= desired, Message: msg}
}
