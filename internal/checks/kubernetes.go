package checks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/uptimy/agent/internal/monitor"
)

func init() {
	register(monitor.TypeKubernetes, func(ctx context.Context, c *Checker, m monitor.Check) Outcome {
		if c.kube == nil {
			return Outcome{Message: "not running inside Kubernetes (no service account found)"}
		}
		return c.kube.Check(ctx, m.Target)
	})
}

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// KubeClient is a minimal read-only Kubernetes API client using the pod's
// service account. It avoids client-go to keep the binary small.
type KubeClient struct {
	baseURL   string
	tokenPath string
	client    *http.Client

	mu        sync.Mutex
	token     string
	tokenRead time.Time
}

// NewInClusterKubeClient returns a client when running in a pod, or nil.
func NewInClusterKubeClient() *KubeClient {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil
	}
	ca, err := os.ReadFile(serviceAccountDir + "/ca.crt")
	if err != nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{RootCAs: pool}
	return &KubeClient{
		baseURL:   "https://" + net.JoinHostPort(host, port),
		tokenPath: serviceAccountDir + "/token",
		client:    &http.Client{Transport: t},
	}
}

// bearer returns the service account token, re-reading it periodically
// because projected tokens rotate.
func (k *KubeClient) bearer() (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.token == "" || time.Since(k.tokenRead) > time.Minute {
		b, err := os.ReadFile(k.tokenPath)
		if err != nil {
			return "", err
		}
		k.token, k.tokenRead = strings.TrimSpace(string(b)), time.Now()
	}
	return k.token, nil
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

// Check reports whether a workload has all its replicas ready.
func (k *KubeClient) Check(ctx context.Context, target string) Outcome {
	ns, kind, name, err := monitor.ParseKubernetesTarget(target)
	if err != nil {
		return Outcome{Message: err.Error()}
	}
	path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/%ss/%s", ns, kind, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.baseURL+path, nil)
	if err != nil {
		return Outcome{Message: err.Error()}
	}
	token, err := k.bearer()
	if err != nil {
		return Outcome{Message: "reading service account token: " + err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := k.client.Do(req)
	if err != nil {
		return Outcome{Message: trimErr(err)}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Outcome{Message: fmt.Sprintf("%s %s/%s not found", kind, ns, name)}
	case http.StatusForbidden:
		return Outcome{Message: "forbidden: the agent's service account can't read " + kind + "s (check RBAC)"}
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return Outcome{Message: fmt.Sprintf("kubernetes API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
	}

	var w workloadStatus
	if err := json.NewDecoder(resp.Body).Decode(&w); err != nil {
		return Outcome{Message: "decoding response: " + err.Error()}
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
