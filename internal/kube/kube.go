// Package kube is a minimal read-only Kubernetes API client using the pod's
// service account. It avoids client-go to keep the binary small.
package kube

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// Client reads from the Kubernetes API.
type Client struct {
	baseURL   string
	tokenPath string
	client    *http.Client
	namespace string

	mu        sync.Mutex
	token     string
	tokenRead time.Time
}

// NewInCluster returns a client when running in a pod, or nil.
func NewInCluster() *Client {
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
	ns, _ := os.ReadFile(serviceAccountDir + "/namespace")
	return &Client{
		baseURL:   "https://" + net.JoinHostPort(host, port),
		tokenPath: serviceAccountDir + "/token",
		client:    &http.Client{Transport: t},
		namespace: strings.TrimSpace(string(ns)),
	}
}

// New returns a client for baseURL that sends no credentials, for tests.
func New(baseURL, namespace string) *Client {
	return &Client{baseURL: baseURL, client: http.DefaultClient, namespace: namespace}
}

// Namespace is the namespace the agent's pod runs in.
func (k *Client) Namespace() string { return k.namespace }

// bearer returns the service account token, re-reading it periodically
// because projected tokens rotate.
func (k *Client) bearer() (string, error) {
	if k.tokenPath == "" {
		return "", nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.token == "" || time.Since(k.tokenRead) > time.Minute {
		b, err := os.ReadFile(k.tokenPath)
		if err != nil {
			return "", fmt.Errorf("reading service account token: %w", err)
		}
		k.token, k.tokenRead = strings.TrimSpace(string(b)), time.Now()
	}
	return k.token, nil
}

// StatusError is a response other than 200 from the API.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("kubernetes API returned %d: %s", e.Code, e.Body)
}

// IsNotFound reports whether err is a 404: the object, or the resource type
// (a CRD that isn't installed), doesn't exist.
func IsNotFound(err error) bool { return statusIs(err, http.StatusNotFound) }

// IsForbidden reports whether err is a 403: RBAC doesn't allow it.
func IsForbidden(err error) bool { return statusIs(err, http.StatusForbidden) }

func statusIs(err error, code int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == code
}

// Get decodes the JSON at path (e.g. /apis/apps/v1/namespaces/x/deployments/y)
// into out.
func (k *Client) Get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.baseURL+path, nil)
	if err != nil {
		return err
	}
	token, err := k.bearer()
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := k.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return &StatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
