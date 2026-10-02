package checks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uptimy/agent/internal/kube"
	"github.com/uptimy/agent/internal/monitor"
)

func TestKubernetesService(t *testing.T) {
	responses := map[string]string{
		// Dual-stack: the pod "a" appears in both slices.
		"/apis/discovery.k8s.io/v1/namespaces/shop/endpointslices?labelSelector=kubernetes.io%2Fservice-name%3Dweb": `{"items":[
			{"endpoints":[{"addresses":["10.0.0.1"],"conditions":{"ready":true},"targetRef":{"uid":"a"}},
			              {"addresses":["10.0.0.2"],"conditions":{"ready":false},"targetRef":{"uid":"b"}}]},
			{"endpoints":[{"addresses":["fd00::1"],"conditions":{"ready":true},"targetRef":{"uid":"a"}}]}]}`,
		"/apis/discovery.k8s.io/v1/namespaces/shop/endpointslices?labelSelector=kubernetes.io%2Fservice-name%3Ddown": `{"items":[
			{"endpoints":[{"addresses":["10.0.0.3"],"conditions":{"ready":false}}]}]}`,
		"/apis/discovery.k8s.io/v1/namespaces/shop/endpointslices?labelSelector=kubernetes.io%2Fservice-name%3Dempty": `{"items":[]}`,
		"/api/v1/namespaces/shop/services/empty": `{"spec":{"type":"ClusterIP"}}`,
		"/apis/discovery.k8s.io/v1/namespaces/shop/endpointslices?labelSelector=kubernetes.io%2Fservice-name%3Dgone": `{"items":[]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[r.URL.RequestURI()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	c := New(kube.New(srv.URL, ""))

	for target, want := range map[string]Outcome{
		"shop/service/web":   {OK: true, Message: "1/2 endpoints ready"},
		"shop/service/down":  {Message: "0/1 endpoints ready"},
		"shop/service/empty": {Message: "no endpoints: no running pod matches the Service's selector"},
		"shop/service/gone":  {Message: "service shop/gone not found"},
	} {
		got := c.Run(context.Background(), monitor.Check{Type: monitor.TypeKubernetes, Target: target})
		if got.OK != want.OK || got.Message != want.Message {
			t.Errorf("%s: got %+v, want %+v", target, got, want)
		}
	}
}
