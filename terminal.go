package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"antek-async/internal/kube"

	"github.com/gorilla/websocket"
	utilsexec "k8s.io/client-go/util/exec"
)

// terminalPath is the only route the loopback listener serves.
const terminalPath = "/terminal"

const (
	// terminalTicketTTL bounds the gap between the drawer asking for an endpoint and connecting
	// to it. It is short because the ticket is a capability to run a shell in a pod.
	terminalTicketTTL = 30 * time.Second

	// terminalPingPeriod and terminalPongWait keep a quiet session alive and notice a drawer
	// that went away without closing the socket.
	terminalPingPeriod = 30 * time.Second
	terminalPongWait   = 70 * time.Second
	terminalWriteWait  = 10 * time.Second

	// terminalReadLimit caps one inbound frame. Typed input is tiny, but a paste is not, and
	// an unbounded frame would be an unbounded allocation.
	terminalReadLimit = 1 << 20
)

// The two control messages the drawer sends as text. Everything else it sends is a binary frame
// of the bytes the user typed.
const (
	terminalInputResize = "resize"
	terminalInputClose  = "close"
)

// TerminalEndpoint is what the drawer needs to open a session: where to connect, and which
// container actually answered, so the picker cannot show one container while the session runs in
// another.
type TerminalEndpoint struct {
	URL       string `json:"url"`
	Container string `json:"container"`
}

// terminalTicket is one prepared session. The kubeconfig path is captured with it so a session
// keeps the cluster it was started against even if the user switches kubeconfig while the drawer
// is still connecting.
type terminalTicket struct {
	path      string
	request   kube.TerminalRequest
	expiresAt time.Time
}

// terminalReady is the first message the drawer sees: the socket is up, so the terminal can stop
// showing that it is connecting.
type terminalReady struct {
	Type string `json:"type"`
}

// terminalExit is how a session finishes. Code is the command's exit code, and Reason carries the
// cluster's own message when the session did not end by exiting.
type terminalExit struct {
	Type   string `json:"type"`
	Code   int    `json:"code"`
	Reason string `json:"reason"`
}

// terminalInput is the shape of the two text messages the drawer sends.
type terminalInput struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// terminalRunner opens the cluster side of a session. It is a field rather than a direct call so
// the frame protocol can be exercised by a test without a cluster.
type terminalRunner func(ctx context.Context, path string, request kube.TerminalRequest, streams kube.TermStreams) error

// terminalServer serves the terminal WebSocket and owns its listener. The listener is created on
// first use, so a user who never opens a terminal never has a port open.
type terminalServer struct {
	run terminalRunner

	// ctx is the parent of every session, so close ends them all at once.
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once

	mu        sync.Mutex
	listener  net.Listener
	server    *http.Server
	url       string
	listenErr error
	closed    bool
	tickets   map[string]terminalTicket
	sessions  map[*terminalSession]struct{}
}

func newTerminalServer() *terminalServer {
	ctx, cancel := context.WithCancel(context.Background())

	return &terminalServer{
		run:      kube.ExecTerminal,
		ctx:      ctx,
		cancel:   cancel,
		tickets:  map[string]terminalTicket{},
		sessions: map[*terminalSession]struct{}{},
	}
}

// ensure starts the loopback listener the first time a terminal is asked for, and returns the
// endpoint to connect to. Binding port 0 lets the kernel pick a free port, so two copies of the
// app never collide.
func (s *terminalServer) ensure() (string, error) {
	s.once.Do(func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			s.listenErr = fmt.Errorf("cannot start the terminal listener: %w", err)
			return
		}

		mux := http.NewServeMux()
		mux.HandleFunc(terminalPath, s.handle)

		s.mu.Lock()
		s.listener = listener
		s.server = &http.Server{Handler: mux, ReadHeaderTimeout: terminalWriteWait}
		s.url = "ws://" + listener.Addr().String() + terminalPath
		server := s.server
		s.mu.Unlock()

		go func() {
			// The listener is loopback only and serves one route, so a serve that stops is
			// worth a log line and nothing more.
			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("terminal listener stopped: %v", err)
			}
		}()
	})

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listenErr != nil {
		return "", s.listenErr
	}

	return s.url, nil
}

// prepare validates a request against the cluster and mints the one-time endpoint that starts it.
// The cluster is asked here rather than on connect, so a container the pod does not declare
// reaches the drawer as a plain error instead of a socket that fails right after opening.
func (s *terminalServer) prepare(ctx context.Context, path string, request kube.TerminalRequest) (TerminalEndpoint, error) {
	if path == "" {
		return TerminalEndpoint{}, errors.New("Kubeconfig not found")
	}

	if request.Namespace == "" || request.Pod == "" {
		return TerminalEndpoint{}, errors.New("Namespace and pod are required")
	}

	clientset, err := kube.ClientFor(path)
	if err != nil {
		return TerminalEndpoint{}, err
	}

	containers, err := kube.PodContainers(ctx, clientset, request.Namespace, request.Pod)
	if err != nil {
		return TerminalEndpoint{}, err
	}

	container, err := kube.ResolveContainer(containers, request.Container)
	if err != nil {
		return TerminalEndpoint{}, err
	}
	request.Container = container

	endpoint, err := s.ensure()
	if err != nil {
		return TerminalEndpoint{}, err
	}

	token, err := s.issue(path, request, time.Now().Add(terminalTicketTTL))
	if err != nil {
		return TerminalEndpoint{}, err
	}

	return TerminalEndpoint{URL: endpoint + "?id=" + token, Container: container}, nil
}

// issue stores a prepared session under a fresh token. The token is what allows exactly one
// connection: the listener is on loopback, but every local process can reach loopback, so without
// it another program could run a shell in a pod using this app's credentials.
func (s *terminalServer) issue(path string, request kube.TerminalRequest, expiresAt time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("cannot create a terminal ticket: %w", err)
	}

	token := hex.EncodeToString(raw)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.expireTicketsLocked(time.Now())
	s.tickets[token] = terminalTicket{
		path:      path,
		request:   request,
		expiresAt: expiresAt,
	}

	return token, nil
}

// expireTicketsLocked drops tickets whose drawer never connected. It runs when a ticket is added
// rather than on a timer, so the server keeps no background goroutine. Callers must hold s.mu.
func (s *terminalServer) expireTicketsLocked(now time.Time) {
	for token, ticket := range s.tickets {
		if now.After(ticket.expiresAt) {
			delete(s.tickets, token)
		}
	}
}

// consume spends a ticket, so an endpoint that leaked cannot be replayed.
func (s *terminalServer) consume(token string) (terminalTicket, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ticket, ok := s.tickets[token]
	if !ok {
		return terminalTicket{}, false
	}
	delete(s.tickets, token)

	if time.Now().After(ticket.expiresAt) {
		return terminalTicket{}, false
	}

	return ticket, true
}

// terminalUpgrader upgrades one request into a session. CheckOrigin has to be set here: the
// gorilla default compares the Origin host with the request Host, and the webview reports a
// wails:// origin while the request goes to 127.0.0.1, so the default would refuse every
// handshake. The check below is defence in depth; the single-use ticket is the real gate.
var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return allowedTerminalOrigin(r.Header.Get("Origin"))
	},
	Error: func(_ http.ResponseWriter, r *http.Request, status int, reason error) {
		// The observed origin is logged because it cannot be derived from the source: each
		// platform reports its own, and this line is how a refused handshake is diagnosed.
		log.Printf("terminal handshake refused (origin %q, host %q): %d %v", r.Header.Get("Origin"), r.Host, status, reason)
	},
}

// allowedTerminalOrigin accepts the origins a desktop webview can report: the wails:// scheme the
// packaged app uses, and any loopback host, which is where the Vite dev server runs. A page
// served from anywhere else is refused. An empty origin is allowed because some webviews leave
// the header out, and the ticket already proves this app asked for the session.
func allowedTerminalOrigin(origin string) bool {
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	switch parsed.Scheme {
	case "wails":
		return true
	case "http", "https":
	default:
		return false
	}

	host := parsed.Hostname()

	return host == "localhost" || host == "wails.localhost" || net.ParseIP(host).IsLoopback()
}

// handle turns one request into a session and runs it until either end stops. The request is
// already authenticated by its ticket, and the cluster work is delegated to the runner.
func (s *terminalServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ticket, ok := s.consume(r.URL.Query().Get("id"))
	if !ok {
		// An unknown id is not told apart from an expired one: the drawer only ever uses a
		// ticket it just asked for, so either way there is nothing for the caller to act on.
		http.Error(w, "Unknown terminal session", http.StatusNotFound)
		return
	}

	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already written the refusal, including a rejected origin.
		return
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	defer conn.Close()

	stdin, stdinWriter := io.Pipe()
	defer stdinWriter.Close()

	session := &terminalSession{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
		sizes:  make(chan kube.TermSize, 1),
	}
	if !s.register(session) {
		return
	}
	defer s.unregister(session)

	go session.ping(ctx)
	go session.read(stdinWriter, cancel)

	// The socket is up, which is all that can be reported before the command starts. A request
	// the cluster refuses arrives straight after as an exit message.
	_ = session.control(terminalReady{Type: "ready"})

	session.end(s.run(ctx, ticket.path, ticket.request, kube.TermStreams{
		Stdin:  stdin,
		Stdout: session,
		Sizes:  session.sizes,
	}))
}

func (s *terminalServer) register(session *terminalSession) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false
	}

	s.sessions[session] = struct{}{}

	return true
}

func (s *terminalServer) unregister(session *terminalSession) {
	s.mu.Lock()
	delete(s.sessions, session)
	s.mu.Unlock()
}

// closeSessions ends every live session. A kubeconfig switch needs this: a shell belongs to the
// cluster it was started on, so it must not outlive the switch.
func (s *terminalServer) closeSessions() {
	s.mu.Lock()
	sessions := make([]*terminalSession, 0, len(s.sessions))
	for session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()

	for _, session := range sessions {
		session.close()
	}
}

// close stops the listener and every session, so the process can exit.
func (s *terminalServer) close() {
	s.mu.Lock()
	s.closed = true
	server := s.server
	s.mu.Unlock()

	s.cancel()
	s.closeSessions()

	if server != nil {
		_ = server.Close()
	}
}

// terminalSession is one live connection: the socket, the queue of size changes, and the lock
// that keeps the frame writer single. It is registered while it runs so a kubeconfig switch or a
// shutdown can end it, because the cluster connection outlives the request that opened it.
type terminalSession struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	sizes  chan kube.TermSize
	// write keeps the frame writer single, which gorilla requires: only one goroutine may be
	// writing at a time, and stdout and stderr share this writer.
	write sync.Mutex
}

// Write sends one chunk of the session's output as a binary frame. A WebSocket write is all or
// nothing, so it reports either the whole buffer or the error that ended the session. Output is
// never sent as text because a read can split a multi-byte character, and only raw bytes survive
// that intact.
func (s *terminalSession) Write(p []byte) (int, error) {
	if err := s.frame(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

// control sends one JSON control message.
func (s *terminalSession) control(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return s.frame(websocket.TextMessage, payload)
}

func (s *terminalSession) frame(messageType int, payload []byte) error {
	s.write.Lock()
	defer s.write.Unlock()

	return s.conn.WriteMessage(messageType, payload)
}

// read owns stdin for as long as the socket is open. A binary frame is the bytes the user typed
// and a text frame is one of the two control messages the drawer sends. Returning closes the pipe
// it was given, which is how the command sees its stdin end.
func (s *terminalSession) read(stdin *io.PipeWriter, cancel context.CancelFunc) {
	defer stdin.Close()

	s.conn.SetReadLimit(terminalReadLimit)
	_ = s.conn.SetReadDeadline(time.Now().Add(terminalPongWait))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(terminalPongWait))
	})

	for {
		messageType, payload, err := s.conn.ReadMessage()
		if err != nil {
			cancel()
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			if _, err := stdin.Write(payload); err != nil {
				cancel()
				return
			}
		case websocket.TextMessage:
			var message terminalInput
			if err := json.Unmarshal(payload, &message); err != nil {
				continue
			}

			switch message.Type {
			case terminalInputResize:
				s.resize(message.Cols, message.Rows)
			case terminalInputClose:
				cancel()
				return
			}
		}
	}
}

// resize queues a size change without ever blocking the reader. Dropping one is harmless because
// the next change wins, and the alternative is a reader that stalls behind a full queue.
func (s *terminalSession) resize(cols, rows uint16) {
	if cols == 0 || rows == 0 {
		return
	}

	select {
	case s.sizes <- kube.TermSize{Cols: cols, Rows: rows}:
	default:
	}
}

// ping keeps the session alive while it is quiet and notices a drawer that vanished. WriteControl
// is safe alongside the data writer, which is why it is used instead of a data frame.
func (s *terminalSession) ping(ctx context.Context) {
	ticker := time.NewTicker(terminalPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(terminalWriteWait)); err != nil {
				return
			}
		}
	}
}

// end reports why the session finished, then closes the socket politely so the drawer sees a
// clean end rather than a connection error. The reason is the cluster's own message, which is
// what the user needs for the cases that actually happen: a denied pods/exec rule, or an image
// with no shell in it.
func (s *terminalSession) end(err error) {
	exit := terminalExit{Type: "exit"}

	switch {
	// A session the drawer ended is not a failure, so the context error that stopped the stream
	// is not passed on: closing the terminal is the ordinary way one finishes.
	case s.ctx.Err() != nil:
	case err != nil:
		exit.Code = -1

		var exitErr utilsexec.ExitError
		if errors.As(err, &exitErr) {
			exit.Code = exitErr.ExitStatus()
		}
		exit.Reason = err.Error()
	}

	_ = s.control(exit)
	_ = s.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(terminalWriteWait),
	)
}

// close ends the session from this side: cancelling stops the cluster stream, and closing the
// socket unblocks the reader as well as any write that is waiting on a drawer that stopped
// reading.
func (s *terminalSession) close() {
	s.cancel()
	_ = s.conn.Close()
}
