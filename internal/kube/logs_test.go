package kube

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestPodLogOptions(t *testing.T) {
	tests := []struct {
		name      string
		request   LogRequest
		wantTail  bool
		wantLines int64
	}{
		{
			// A missing TailLines is what makes the API server send the whole log.
			name:    "a zero tail asks for the whole log",
			request: LogRequest{Container: "app"},
		},
		{
			name:      "a positive tail is sent",
			request:   LogRequest{Container: "app", Follow: true, TailLines: 200},
			wantTail:  true,
			wantLines: 200,
		},
		{
			name:    "a negative tail asks for the whole log",
			request: LogRequest{Container: "app", TailLines: -1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := podLogOptions(test.request)

			if options.Container != test.request.Container {
				t.Errorf("container = %q, want %q", options.Container, test.request.Container)
			}
			if options.Follow != test.request.Follow {
				t.Errorf("follow = %v, want %v", options.Follow, test.request.Follow)
			}

			if !test.wantTail {
				if options.TailLines != nil {
					t.Errorf("TailLines = %d, want nil", *options.TailLines)
				}
				return
			}

			if options.TailLines == nil {
				t.Fatal("TailLines = nil, want a value")
			}
			if *options.TailLines != test.wantLines {
				t.Errorf("TailLines = %d, want %d", *options.TailLines, test.wantLines)
			}
		})
	}
}

// logServer serves one canned log response, so the endpoint and the copy are exercised without a
// cluster. The path and the query are checked because both are the contract with the API server;
// every test below asks for the same container, follow and tail, so the helper checks them once.
func logServer(t *testing.T, path, body string, status int) kubernetes.Interface {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("requested %s, want %s", r.URL.Path, path)
		}
		if got := r.URL.Query().Get("container"); got != "app" {
			t.Errorf("container = %q, want %q", got, "app")
		}
		if got := r.URL.Query().Get("follow"); got != "true" {
			t.Errorf("follow = %q, want %q", got, "true")
		}
		if got := r.URL.Query().Get("tailLines"); got != "200" {
			t.Errorf("tailLines = %q, want %q", got, "200")
		}

		w.Header().Set("Content-Type", "text/plain")
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

func TestPodLogsCopiesTheStream(t *testing.T) {
	body := "hello\nworld\n"
	clientset := logServer(t, "/api/v1/namespaces/shop/pods/web-0/log", body, http.StatusOK)

	request := LogRequest{Namespace: "shop", Pod: "web-0", Container: "app", Follow: true, TailLines: 200}

	var out bytes.Buffer
	if err := PodLogs(context.Background(), clientset, request, &out); err != nil {
		t.Fatalf("PodLogs() = %v, want nil", err)
	}
	if out.String() != body {
		t.Errorf("stream = %q, want %q", out.String(), body)
	}
}

func TestPodLogsReportsAnError(t *testing.T) {
	body := `{"kind":"Status","message":"a container name must be specified for pod web-0","code":400}`
	clientset := logServer(t, "/api/v1/namespaces/shop/pods/web-0/log", body, http.StatusBadRequest)

	request := LogRequest{Namespace: "shop", Pod: "web-0", Container: "app", Follow: true, TailLines: 200}

	err := PodLogs(context.Background(), clientset, request, io.Discard)
	if err == nil {
		t.Fatal("PodLogs() = nil, want an error")
	}
	// The message is shown to the user, so it has to name the pod the log was asked for.
	if !strings.Contains(err.Error(), "cannot read logs for Pod shop/web-0") {
		t.Errorf("error %q does not name the pod", err)
	}
}
