package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"antek-async/internal/kube"

	"github.com/gorilla/websocket"
	utilsexec "k8s.io/client-go/util/exec"
)

func TestAllowedTerminalOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		// Some webviews leave the header out, and the ticket already proves the app asked.
		{origin: "", want: true},
		{origin: "wails://wails", want: true},
		{origin: "http://wails.localhost", want: true},
		{origin: "http://localhost:34115", want: true},
		{origin: "http://127.0.0.1:34115", want: true},
		{origin: "https://example.com", want: false},
		{origin: "http://evil.test", want: false},
		{origin: "file:///tmp/session", want: false},
	}

	for _, test := range tests {
		t.Run(test.origin, func(t *testing.T) {
			if got := allowedTerminalOrigin(test.origin); got != test.want {
				t.Errorf("allowedTerminalOrigin(%q) = %v, want %v", test.origin, got, test.want)
			}
		})
	}
}

func TestTerminalTicketIsSpentOnce(t *testing.T) {
	server := newTerminalServer()
	defer server.close()

	token, err := server.issue("/tmp/kubeconfig", kube.TerminalRequest{Pod: "web-0"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("cannot issue a ticket: %v", err)
	}

	if _, ok := server.consume(token); !ok {
		t.Fatal("the first connection was refused")
	}
	if _, ok := server.consume(token); ok {
		t.Fatal("the same ticket was accepted twice")
	}
}
func TestTerminalTicketExpires(t *testing.T) {
	server := newTerminalServer()
	defer server.close()

	token, err := server.issue("/tmp/kubeconfig", kube.TerminalRequest{Pod: "web-0"}, time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("cannot issue a ticket: %v", err)
	}

	if _, ok := server.consume(token); ok {
		t.Fatal("an expired ticket was accepted")
	}
}

func TestTerminalHandshakeRefusesAnUnknownTicket(t *testing.T) {
	server := newTerminalServer()
	defer server.close()

	endpoint, err := server.ensure()
	if err != nil {
		t.Fatalf("cannot start the terminal listener: %v", err)
	}

	conn, response, err := websocket.DefaultDialer.Dial(endpoint+"?id=made-up", nil)
	if err == nil {
		conn.Close()
		t.Fatal("a made-up ticket opened a session")
	}
	if response == nil {
		t.Fatal("the handshake returned no response to inspect")
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("handshake status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func TestTerminalSessionEchoesWhatIsTyped(t *testing.T) {
	server := newTerminalServer()
	defer server.close()

	sizes := make(chan kube.TermSize, 1)
	server.run = func(ctx context.Context, path string, request kube.TerminalRequest, streams kube.TermStreams) error {
		if path != "/tmp/kubeconfig" {
			t.Errorf("session ran against %q, want the kubeconfig the ticket captured", path)
		}
		if request.Cols != 100 || request.Rows != 30 {
			t.Errorf("session started at %dx%d, want the size the ticket carried", request.Cols, request.Rows)
		}

		// The size reported after connecting is what a resize becomes.
		go func() {
			select {
			case size := <-streams.Sizes:
				sizes <- size
			case <-ctx.Done():
			}
		}()

		_, err := io.Copy(streams.Stdout, streams.Stdin)
		return err
	}

	conn := terminalTestClient(t, server)

	if ready := readTerminalText(t, conn); ready.Type != "ready" {
		t.Fatalf("first message = %+v, want a ready message", ready)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":120,"rows":40}`)); err != nil {
		t.Fatalf("cannot send a resize: %v", err)
	}

	select {
	case size := <-sizes:
		if size.Cols != 120 || size.Rows != 40 {
			t.Errorf("resize reached the session as %dx%d, want 120x40", size.Cols, size.Rows)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the resize never reached the session")
	}

	// Typed input travels as bytes so a multi-byte character split across frames survives.
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("echo me")); err != nil {
		t.Fatalf("cannot send input: %v", err)
	}

	messageType, payload := readTerminalFrame(t, conn)
	if messageType != websocket.BinaryMessage {
		t.Fatalf("output arrived as frame type %d, want binary", messageType)
	}
	if string(payload) != "echo me" {
		t.Errorf("output = %q, want %q", payload, "echo me")
	}

	// Closing the session is how the drawer ends it, which is not a failure.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"close"}`)); err != nil {
		t.Fatalf("cannot close the session: %v", err)
	}

	exit := readTerminalText(t, conn)
	if exit.Type != "exit" || exit.Code != 0 || exit.Reason != "" {
		t.Errorf("exit = %+v, want a clean exit", exit)
	}
}

func TestTerminalSessionReportsTheExitCode(t *testing.T) {
	server := newTerminalServer()
	defer server.close()

	server.run = func(context.Context, string, kube.TerminalRequest, kube.TermStreams) error {
		return utilsexec.CodeExitError{
			Err:  errors.New("command terminated with exit code 7"),
			Code: 7,
		}
	}

	conn := terminalTestClient(t, server)

	if ready := readTerminalText(t, conn); ready.Type != "ready" {
		t.Fatalf("first message = %+v, want a ready message", ready)
	}

	exit := readTerminalText(t, conn)
	if exit.Code != 7 {
		t.Errorf("exit code = %d, want 7", exit.Code)
	}
	if !strings.Contains(exit.Reason, "exit code 7") {
		t.Errorf("exit reason = %q, want the cluster's own message", exit.Reason)
	}
}

// terminalTestClient prepares a session on the test server and connects to it, so a test holds
// the socket the drawer would have held.
func terminalTestClient(t *testing.T, server *terminalServer) *websocket.Conn {
	t.Helper()

	endpoint, err := server.ensure()
	if err != nil {
		t.Fatalf("cannot start the terminal listener: %v", err)
	}

	token, err := server.issue("/tmp/kubeconfig", kube.TerminalRequest{
		Namespace: "default",
		Pod:       "web-0",
		Container: "app",
		Cols:      100,
		Rows:      30,
	}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("cannot issue a ticket: %v", err)
	}

	conn, response, err := websocket.DefaultDialer.Dial(endpoint+"?id="+token, nil)
	if err != nil {
		t.Fatalf("cannot connect to the terminal endpoint: %v", err)
	}
	if response != nil {
		response.Body.Close()
	}

	t.Cleanup(func() { conn.Close() })

	return conn
}

// readTerminalFrame reads one frame, failing the test rather than blocking when the session has
// stopped saying anything.
func readTerminalFrame(t *testing.T, conn *websocket.Conn) (int, []byte) {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("cannot set the read deadline: %v", err)
	}

	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("cannot read a terminal frame: %v", err)
	}

	return messageType, payload
}

// readTerminalText reads one control message, which is the only thing the session sends as text.
func readTerminalText(t *testing.T, conn *websocket.Conn) terminalExit {
	t.Helper()

	messageType, payload := readTerminalFrame(t, conn)
	if messageType != websocket.TextMessage {
		t.Fatalf("control frame arrived as type %d, want text", messageType)
	}

	// The ready and exit messages only differ by their type field, so one struct reads both.
	var message terminalExit
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("control frame = %q, want JSON: %v", payload, err)
	}

	return message
}
