package kube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// metricsServer serves one canned metrics response, so the REST call and the decode are
// exercised without a cluster. The path is checked because the endpoint is the contract with
// metrics-server.
func metricsServer(t *testing.T, path, body string, status int) kubernetes.Interface {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("requested %s, want %s", r.URL.Path, path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	return clientset
}

func TestSumUsage(t *testing.T) {
	container := func(cpu, memory string) containerMetrics {
		return containerMetrics{Usage: metricsUsage{CPU: cpu, Memory: memory}}
	}

	tests := []struct {
		name       string
		containers []containerMetrics
		wantCPU    string
		wantMemory string
	}{
		{
			name:       "no containers",
			containers: nil,
			wantCPU:    "0m",
			wantMemory: "0Mi",
		},
		{
			// The same numbers kubectl's own printer test uses: 0.2 + 0.2 cores, 1Gi + 1Gi.
			name:       "containers are summed",
			containers: []containerMetrics{container("0.2", "1Gi"), container("0.2", "1Gi")},
			wantCPU:    "400m",
			wantMemory: "2048Mi",
		},
		{
			name:       "whole cores stay in millicores",
			containers: []containerMetrics{container("1", "1Gi")},
			wantCPU:    "1000m",
			wantMemory: "1024Mi",
		},
		{
			name:       "memory is rounded down to mebibytes",
			containers: []containerMetrics{container("7m", "1536Ki")},
			wantCPU:    "7m",
			wantMemory: "1Mi",
		},
		{
			name:       "a container that reports nothing is ignored",
			containers: []containerMetrics{container("not-a-quantity", ""), container("5m", "2Mi")},
			wantCPU:    "5m",
			wantMemory: "2Mi",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cpu, memory := sumUsage(test.containers)
			if cpu != test.wantCPU {
				t.Errorf("cpu = %q, want %q", cpu, test.wantCPU)
			}
			if memory != test.wantMemory {
				t.Errorf("memory = %q, want %q", memory, test.wantMemory)
			}
		})
	}
}

func TestPodUsages(t *testing.T) {
	body := `{
		"kind": "PodMetricsList",
		"items": [
			{
				"metadata": {"name": "web", "namespace": "shop"},
				"containers": [
					{"name": "app", "usage": {"cpu": "200m", "memory": "1Gi"}},
					{"name": "sidecar", "usage": {"cpu": "200m", "memory": "1Gi"}}
				]
			},
			{
				"metadata": {"name": "api", "namespace": "shop"},
				"containers": [{"name": "app", "usage": {"cpu": "1", "memory": "64Mi"}}]
			},
			{
				"metadata": {"name": "cache", "namespace": "data"},
				"containers": [{"name": "redis", "usage": {"cpu": "3m", "memory": "120Mi"}}]
			}
		]
	}`

	clientset := metricsServer(t, "/apis/metrics.k8s.io/v1beta1/pods", body, http.StatusOK)

	got, err := PodUsages(context.Background(), clientset)
	if err != nil {
		t.Fatalf("PodUsages() = %v, want nil", err)
	}

	want := []PodUsage{
		{Namespace: "data", Name: "cache", CPU: "3m", Memory: "120Mi"},
		{Namespace: "shop", Name: "api", CPU: "1000m", Memory: "64Mi"},
		{Namespace: "shop", Name: "web", CPU: "400m", Memory: "2048Mi"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d usages, want %d", len(got), len(want))
	}
	for i, usage := range want {
		if got[i] != usage {
			t.Errorf("usage %d = %+v, want %+v", i, got[i], usage)
		}
	}
}

func TestNodeUsages(t *testing.T) {
	body := `{
		"kind": "NodeMetricsList",
		"items": [
			{"metadata": {"name": "worker-1"}, "usage": {"cpu": "250m", "memory": "3Gi"}},
			{"metadata": {"name": "master-1"}, "usage": {"cpu": "1", "memory": "1Gi"}}
		]
	}`

	clientset := metricsServer(t, "/apis/metrics.k8s.io/v1beta1/nodes", body, http.StatusOK)

	got, err := NodeUsages(context.Background(), clientset)
	if err != nil {
		t.Fatalf("NodeUsages() = %v, want nil", err)
	}

	want := []NodeUsage{
		{Name: "master-1", CPU: "1000m", Memory: "1024Mi"},
		{Name: "worker-1", CPU: "250m", Memory: "3072Mi"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d usages, want %d", len(got), len(want))
	}
	for i, usage := range want {
		if got[i] != usage {
			t.Errorf("usage %d = %+v, want %+v", i, got[i], usage)
		}
	}
}

func TestUsagesReportUnavailableMetrics(t *testing.T) {
	body := `{"kind":"Status","message":"the server could not find the requested resource"}`

	if _, err := PodUsages(context.Background(), metricsServer(t, "/apis/metrics.k8s.io/v1beta1/pods", body, http.StatusNotFound)); err == nil {
		t.Error("PodUsages() = nil, want an error")
	} else if !strings.Contains(err.Error(), "Pod metrics") {
		t.Errorf("error %q does not say which column failed", err)
	}

	if _, err := NodeUsages(context.Background(), metricsServer(t, "/apis/metrics.k8s.io/v1beta1/nodes", body, http.StatusNotFound)); err == nil {
		t.Error("NodeUsages() = nil, want an error")
	} else if !strings.Contains(err.Error(), "Node metrics") {
		t.Errorf("error %q does not say which column failed", err)
	}
}

func TestUsagesReportMalformedMetrics(t *testing.T) {
	clientset := metricsServer(t, "/apis/metrics.k8s.io/v1beta1/pods", `{"items": "not a list"}`, http.StatusOK)

	if _, err := PodUsages(context.Background(), clientset); err == nil {
		t.Error("PodUsages() = nil, want an error")
	}
}
