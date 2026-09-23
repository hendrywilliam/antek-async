package kube

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/tools/remotecommand"
)

func TestResolveContainer(t *testing.T) {
	tests := []struct {
		name       string
		containers []string
		wanted     string
		want       string
		wantErr    string
	}{
		{
			name:       "an empty request takes the first container",
			containers: []string{"app", "sidecar"},
			want:       "app",
		},
		{
			name:       "a declared container is kept",
			containers: []string{"app", "sidecar"},
			wanted:     "sidecar",
			want:       "sidecar",
		},
		{
			name:       "a container the pod does not declare is refused",
			containers: []string{"app"},
			wanted:     "istio-proxy",
			wantErr:    "istio-proxy",
		},
		{
			name:    "a pod without containers is refused",
			wanted:  "app",
			wantErr: "no containers",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveContainer(test.containers, test.wanted)
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("ResolveContainer(%v, %q) = %q, want an error", test.containers, test.wanted, got)
				}
				// The message is shown to the user, so it has to name what is missing.
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("error %q does not mention %q", err, test.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveContainer(%v, %q) returned %v", test.containers, test.wanted, err)
			}
			if got != test.want {
				t.Errorf("ResolveContainer(%v, %q) = %q, want %q", test.containers, test.wanted, got, test.want)
			}
		})
	}
}

func TestTerminalCommandDefaultsToShell(t *testing.T) {
	if got, want := (TerminalRequest{}).command(), []string{"/bin/sh"}; !reflect.DeepEqual(got, want) {
		t.Errorf("command() = %v, want %v", got, want)
	}

	named := TerminalRequest{Command: []string{"bash", "-l"}}
	if got, want := named.command(), []string{"bash", "-l"}; !reflect.DeepEqual(got, want) {
		t.Errorf("command() = %v, want %v", got, want)
	}
}

func TestSizeQueueSeedsTheInitialSize(t *testing.T) {
	// The PTY is allocated before any output can arrive, so the very first Next has to answer
	// without waiting for a resize the drawer may never send.
	queue := newSizeQueue(context.Background(), 120, 40, make(chan TermSize))

	first := queue.Next()
	if first == nil {
		t.Fatal("Next() = nil, want the size the session started with")
	}
	if first.Width != 120 || first.Height != 40 {
		t.Errorf("Next() = %dx%d, want 120x40", first.Width, first.Height)
	}
}

func TestSizeQueueSkipsZeroSizes(t *testing.T) {
	sizes := make(chan TermSize, 2)
	sizes <- TermSize{Cols: 0, Rows: 40}
	sizes <- TermSize{Cols: 80, Rows: 24}

	// No seed, so the first Next is a change the drawer reported after connecting.
	queue := newSizeQueue(context.Background(), 0, 0, sizes)

	next := queue.Next()
	if next == nil {
		t.Fatal("Next() = nil, want the size that followed the collapsed one")
	}
	if next.Width != 80 || next.Height != 24 {
		t.Errorf("Next() = %dx%d, want 80x24", next.Width, next.Height)
	}
}

func TestSizeQueueStopsWhenTheSessionEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The channel stays empty, so Next is parked when the session ends.
	queue := newSizeQueue(ctx, 80, 24, make(chan TermSize))
	if seed := queue.Next(); seed == nil {
		t.Fatal("Next() = nil, want the size the session started with")
	}

	sizes := make(chan *remotecommand.TerminalSize, 1)
	go func() { sizes <- queue.Next() }()

	cancel()

	select {
	case size := <-sizes:
		if size != nil {
			t.Fatalf("Next() after the session ended = %dx%d, want nil", size.Width, size.Height)
		}
	case <-time.After(time.Second):
		t.Fatal("Next() did not return after the session ended")
	}
}
