// Package daemon provides an IPC server that exposes a RESTful API
// over a Unix domain socket for CLI ↔ daemon communication.
//
// Security model (see ADR-003):
//   - Instance ownership via flock on devtether.lock (primary authority).
//   - PID file (devtether.pid) for diagnostics and SIGTERM fallback.
//   - Socket is placed in $XDG_RUNTIME_DIR/devtether/ (per-user, tmpfs-backed).
//   - Secure fallback to /tmp/devtether-<uid>/ when XDG_RUNTIME_DIR is unset.
//   - Directory validated via Lstat + UID ownership check (symlink protection).
//   - Socket permissions are 0600 (owner-only read/write).
//   - Socket directory permissions are 0700.
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/ivin-titus/devtether/internal/config"
	"github.com/ivin-titus/devtether/internal/health"
	"github.com/ivin-titus/devtether/internal/logger"
	"github.com/ivin-titus/devtether/internal/netutil"
	"github.com/ivin-titus/devtether/internal/router"
)

// CheckRunning returns an error if another daemon instance is already active.
func CheckRunning(ctx context.Context) error {
	socketPath := SocketPath()
	if _, statErr := os.Stat(socketPath); statErr == nil {
		dialer := net.Dialer{Timeout: 1 * time.Second}
		conn, dialErr := dialer.DialContext(ctx, "unix", socketPath)
		if dialErr == nil {
			_ = conn.Close()
			return fmt.Errorf("daemon: already running on %s — run 'devtether status' to inspect it or 'devtether down' to stop it", socketPath)
		}
		// Permission denied = socket owned by another user. Don't mask it.
		if netutil.IsPermissionError(dialErr) {
			return fmt.Errorf("daemon: socket permission denied on %s (a root-owned daemon may be running; rerun with sudo): %w", socketPath, dialErr)
		}
		// Connection refused = stale socket from a dead process. Safe to proceed.
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("daemon: failed to pre-flight socket: %w", statErr)
	}
	return nil
}

// SocketPath returns the platform-appropriate path for the IPC socket.
// Uses the same directory resolution as RuntimeDir (see lifecycle.go).
func SocketPath() string {
	return filepath.Join(runtimeDirPath(), "devtether.sock")
}

// Server runs an HTTP API over a Unix domain socket for IPC communication.
type Server struct {
	engine     *router.Engine
	tracker    *health.Tracker
	httpServer *http.Server
	socketPath string
	configPath string
	startTime  time.Time
	cancelFunc context.CancelFunc
	lockFile   *os.File
	nonce      string
}

// NewServer initializes the IPC daemon. configPath is the config file the
// daemon loaded; it is reported by /status so a client can identify which
// project instance it is talking to. tracker holds the route health
// observations shared by the startup check, proxy traffic, and diagnostics.
func NewServer(engine *router.Engine, tracker *health.Tracker, cancel context.CancelFunc, configPath string) *Server {
	return &Server{
		engine:     engine,
		tracker:    tracker,
		cancelFunc: cancel,
		socketPath: SocketPath(),
		configPath: configPath,
	}
}

// Start begins listening on the Unix domain socket. It blocks until the
// context is cancelled or a fatal error occurs.
//
// Startup sequence (ADR-003):
//
//	RuntimeDir validation → flock(LOCK_EX|LOCK_NB) → PID write → stale socket cleanup → bind → serve
func (s *Server) Start(ctx context.Context) error {
	listener, err := s.Listen(ctx)
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

// Listen acquires daemon ownership and binds the IPC socket before any
// concurrent server work begins. The returned listener remains owned by s.
func (s *Server) Listen(ctx context.Context) (net.Listener, error) {
	// 1. Validate runtime directory (creates if needed, checks ownership).
	if _, dirErr := RuntimeDir(); dirErr != nil {
		return nil, dirErr
	}

	// 2. Acquire exclusive instance lock (non-blocking).
	lockFile, lockErr := AcquireLock()
	if lockErr != nil {
		return nil, lockErr
	}
	s.lockFile = lockFile

	// 3. Write PID file (atomic: temp → fsync → rename).
	if pidErr := WritePID(); pidErr != nil {
		_ = s.lockFile.Close()
		s.lockFile = nil
		return nil, pidErr
	}
	nonce, nonceErr := writeNonce()
	if nonceErr != nil {
		RemovePID()
		_ = s.lockFile.Close()
		s.lockFile = nil
		return nil, nonceErr
	}
	s.nonce = nonce

	// 4. Clean up stale socket from a previous crash.
	// Under flock protection, this is safe — no other instance can race us.
	// Lstat (no symlink follow) is required: a *dangling* symlink at the socket
	// path looks absent to os.Stat, so bind would fail later with a misleading
	// "address already in use" instead of an explicit refusal (ADR-003).
	if linfo, statErr := os.Lstat(s.socketPath); statErr == nil {
		if linfo.Mode()&os.ModeSymlink != 0 {
			RemovePID()
			_ = s.lockFile.Close()
			s.lockFile = nil
			return nil, fmt.Errorf("daemon: socket path %s is a symlink (possible attack)", s.socketPath)
		}
		if linfo.Mode()&os.ModeSocket == 0 {
			RemovePID()
			_ = s.lockFile.Close()
			s.lockFile = nil
			return nil, fmt.Errorf("daemon: socket path %s exists but is not a unix socket", s.socketPath)
		}
		if rmErr := os.Remove(s.socketPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			RemovePID()
			_ = s.lockFile.Close()
			s.lockFile = nil
			return nil, fmt.Errorf("daemon: failed to clear stale socket: %w", rmErr)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		RemovePID()
		_ = s.lockFile.Close()
		s.lockFile = nil
		return nil, fmt.Errorf("daemon: failed to stat socket: %w", statErr)
	}

	// 5. Bind the Unix socket.
	var lc net.ListenConfig

	// ADR-003, ADR-009 §3: Atomic 0600 socket creation via umask.
	// The umask is process-global. This is safe because DNS and Proxy listeners
	// are bound synchronously in up.go *before* the errgroup launches Start().
	// If the startup order changes, replace with post-creation os.Chmod.
	oldUmask := syscall.Umask(0177)
	defer syscall.Umask(oldUmask)
	listener, err := lc.Listen(ctx, "unix", s.socketPath)

	if err != nil {
		RemovePID()
		_ = s.lockFile.Close()
		s.lockFile = nil
		return nil, fmt.Errorf("daemon: failed to bind unix socket: %w", err)
	}

	s.startTime = time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/routes", s.handleRoutes)
	mux.HandleFunc("/routes/check", s.handleRoutesCheck)
	mux.HandleFunc("/routes/{domain}", s.handleRouteDomain)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/shutdown", s.handleShutdown)

	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second, // Accommodates the shutdown connection-hold pattern.
		MaxHeaderBytes:    1 << 16,          // 64KB — generous for IPC (ADR-003).
	}
	return listener, nil
}

// Serve accepts IPC connections on listener until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	defer RemovePID()
	defer removeNonce()
	defer func() {
		if s.lockFile != nil {
			_ = s.lockFile.Close()
			s.lockFile = nil
		}
	}()
	// Unlink the socket while the instance lock is still held, so a concurrent
	// start cannot bind a fresh socket that this deferred cleanup then removes.
	if s.socketPath != "" {
		defer func() { _ = os.Remove(s.socketPath) }()
	}

	shutdownComplete := make(chan struct{})

	// Graceful shutdown when context is cancelled.
	//nolint:gosec // Background server goroutine does not need request context
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		close(shutdownComplete)
	}()

	logger.New("daemon").Debug(fmt.Sprintf("ipc listening on %s", s.socketPath))
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownComplete
		return nil
	}
	return fmt.Errorf("daemon: server error: %w", err)
}

// RouteResponse is the JSON representation of a single route.
type RouteResponse struct {
	Domain      string `json:"domain"`
	ServiceName string `json:"serviceName"`
	Port        int    `json:"port"`
	Type        string `json:"type"`
	// Incarnation identifies the current lifetime of the route. It is the
	// authoritative version token for conditional deletion (DELETE
	// /routes/{domain}?inc=N); opaque to clients.
	Incarnation uint64 `json:"incarnation"`
	// Status is the latest observed backend reachability: online, offline,
	// or unknown when no conclusive observation exists.
	Status string `json:"status"`
	// LastChecked is the RFC3339Nano timestamp of the latest observation,
	// empty when none exists.
	LastChecked string `json:"lastChecked,omitempty"`
}

// handleRoutes serves the GET /routes listing and POST /routes creation
// endpoints. Listing has no nonce requirement (read-only diagnostics); the
// mutating POST is handled by handleRouteCreate below via the shared
// dispatch, keeping method checks per-operation.
func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleRoutesList(w, r)
	case http.MethodPost:
		s.handleRouteCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRoutesList(w http.ResponseWriter, r *http.Request) {
	routes := s.engine.GetAllRoutes()
	domains := make([]string, 0, len(routes))
	for _, target := range routes {
		domains = append(domains, target.Domain)
	}
	observations := make(map[string]health.Observation, len(domains))
	for _, res := range s.tracker.Snapshot(domains) {
		observations[res.Domain] = res.Observation
	}

	response := make([]RouteResponse, 0, len(routes))
	for _, target := range routes {
		obs := observations[target.Domain]
		rr := RouteResponse{
			Domain:      target.Domain,
			ServiceName: target.ServiceName,
			Port:        target.Port,
			Type:        string(target.Type),
			Incarnation: target.Incarnation,
			Status:      string(obs.Status),
		}
		if !obs.CheckedAt.IsZero() {
			rr.LastChecked = obs.CheckedAt.Format(time.RFC3339Nano)
		}
		response = append(response, rr)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// CheckResponse is the JSON representation of one route health probe result.
// Latency is TCP connection latency in milliseconds — not HTTP response time.
type CheckResponse struct {
	Domain  string  `json:"domain"`
	Backend string  `json:"backend"`
	Status  string  `json:"status"`
	Latency float64 `json:"latency_ms"`
	Reason  string  `json:"reason,omitempty"`
}

// routeTargets converts engine routes into probe targets on loopback.
func routeTargets(routes []router.RouteView) []health.Target {
	targets := make([]health.Target, 0, len(routes))
	for _, rv := range routes {
		targets = append(targets, health.Target{
			Domain:      rv.Domain,
			Addr:        net.JoinHostPort("127.0.0.1", strconv.Itoa(rv.Port)),
			Incarnation: rv.Incarnation,
		})
	}
	return targets
}

// handleRoutesCheck serves the POST /routes/check endpoint. It runs one
// fresh bounded health-check pass over the daemon's active routes and
// returns structured per-route results. Cancellation follows the request
// context; the check itself enforces health's shared budget and concurrency.
func (s *Server) handleRoutesCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	results := s.tracker.CheckAll(r.Context(), routeTargets(s.engine.GetAllRoutes()))

	response := make([]CheckResponse, 0, len(results))
	for _, res := range results {
		response = append(response, CheckResponse{
			Domain:  res.Domain,
			Backend: res.Addr,
			Status:  string(res.Status),
			Latency: float64(res.Latency) / float64(time.Millisecond),
			Reason:  res.Reason,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// StatusResponse is the JSON representation of daemon status.
type StatusResponse struct {
	PID        int    `json:"pid"`
	Uptime     string `json:"uptime"`
	Routes     int    `json:"routes"`
	MemAlloc   uint64 `json:"mem_alloc"`
	ConfigPath string `json:"config_path"`
}

// handleStatus serves the GET /status endpoint.
// It returns a JSON object containing the daemon's process ID (pid),
// the duration it has been running (uptime), the number of active
// proxy rules (routes), the current heap allocation in bytes (mem_alloc),
// and the config file it loaded (config_path).
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	resp := StatusResponse{
		PID:        os.Getpid(),
		Uptime:     time.Since(s.startTime).Truncate(time.Second).String(),
		Routes:     len(s.engine.GetAllRoutes()),
		MemAlloc:   m.Alloc,
		ConfigPath: s.configPath,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleShutdown serves the POST /shutdown endpoint.
//
// Design: The handler flushes the response immediately and triggers shutdown.
// The HTTP connection closes as soon as this handler returns. The client
// must then poll the daemon's lockfile to determine when the process has
// completely exited.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.checkNonce(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "shutting down"})

	// Flush the response to the client immediately.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Trigger graceful shutdown of all daemon subsystems.
	s.cancelFunc()
}

// checkNonce enforces the shared IPC nonce contract (ADR-003). It writes a
// 403 response and returns false when the header is missing or wrong.
func (s *Server) checkNonce(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("X-Devtether-Nonce") != s.nonce {
		http.Error(w, "invalid IPC nonce", http.StatusForbidden)
		return false
	}
	return true
}

// maxRouteBodyBytes bounds POST /routes bodies; route creation payloads are
// tiny, so anything larger is rejected before parsing (413).
const maxRouteBodyBytes = 8 << 10

// createRouteRequest is the strict JSON shape accepted by POST /routes.
// Exactly these two keys, exact casing; decoded via the Token stream in
// decodeCreateRouteRequest so duplicates, case variants, unknown fields,
// null, non-objects, and trailing data are all rejected.
type createRouteRequest struct {
	Domain string
	Port   int
}

// decodeCreateRouteRequest parses the request body as exactly one JSON
// object {"domain": string, "port": integer} with no duplicate or unknown
// keys and no trailing data. Any deviation is a client error (400).
func decodeCreateRouteRequest(body []byte) (createRouteRequest, error) {
	var req createRouteRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return req, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return req, errNotJSONObject
	}

	seen := make(map[string]bool, 2)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return req, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return req, errNotJSONObject
		}
		if key != "domain" && key != "port" {
			return req, errUnknownField
		}
		if seen[key] {
			return req, errDuplicateField
		}
		seen[key] = true

		valTok, err := dec.Token()
		if err != nil {
			return req, err
		}
		switch key {
		case "domain":
			domain, ok := valTok.(string)
			if !ok {
				return req, errWrongType
			}
			req.Domain = domain
		case "port":
			num, ok := valTok.(json.Number)
			if !ok {
				return req, errWrongType
			}
			port, err := parsePortNumber(num)
			if err != nil {
				return req, err
			}
			req.Port = port
		}
	}

	// Consume the closing brace, then require end of stream.
	if _, err := dec.Token(); err != nil {
		return req, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return req, errTrailingData
		}
		return req, err
	}

	if !seen["domain"] || !seen["port"] {
		return req, errMissingField
	}
	return req, nil
}

// Sentinel decode errors keep handler mapping trivial: every decode failure
// is a 400 regardless of which strictness rule tripped.
var (
	errNotJSONObject  = errors.New("body must be a JSON object")
	errUnknownField   = errors.New("unknown field")
	errDuplicateField = errors.New("duplicate field")
	errWrongType      = errors.New("wrong field type")
	errMissingField   = errors.New("missing required field")
	errTrailingData   = errors.New("trailing data after JSON object")
)

// digitsOnly matches ^[0-9]+$ — rejects signs, fractions, exponents.
var digitsOnly = regexp.MustCompile(`^[0-9]+$`)

// parsePortNumber converts a json.Number port to an int with overflow
// checking. Non-integer forms are rejected before conversion; conversion
// failure is a client error, never a server error.
func parsePortNumber(num json.Number) (int, error) {
	if !digitsOnly.MatchString(num.String()) {
		return 0, errWrongType
	}
	port, err := strconv.Atoi(num.String())
	if err != nil {
		return 0, errWrongType
	}
	return port, nil
}

// handleRouteCreate serves POST /routes: strict validation, atomic engine
// insertion, then tracker registration with the returned incarnation.
func (s *Server) handleRouteCreate(w http.ResponseWriter, r *http.Request) {
	if !s.checkNonce(w, r) {
		return
	}

	// Read the bounded body in full before parsing so any oversized body is
	// a 413 regardless of where a strictness violation would appear.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRouteBodyBytes))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	req, err := decodeCreateRouteRequest(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if err = config.ValidateRoute(req.Domain, req.Port); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	inc, err := s.engine.AddRoute(req.Domain, "live", req.Port, router.RouteLive)
	if err != nil {
		if errors.Is(err, router.ErrRouteExists) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Engine → tracker ordering: register only after the engine accepts the
	// route, keyed by the same normalized domain the engine stores.
	s.tracker.Register(netutil.NormalizeHost(req.Domain), inc)

	resp := RouteResponse{
		Domain:      netutil.NormalizeHost(req.Domain),
		ServiceName: "live",
		Port:        req.Port,
		Type:        string(router.RouteLive),
		Incarnation: inc,
		Status:      string(health.StatusUnknown),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleRouteDomain serves DELETE /routes/{domain}?inc=N. The path domain
// arrives percent-decoded but unnormalized; validation runs on the literal
// value before any engine call. The incarnation query parameter is the
// sole delete condition — there is no unconditional removal path.
func (s *Server) handleRouteDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.checkNonce(w, r) {
		return
	}

	pathDomain := r.PathValue("domain")
	if err := config.ValidateRouteDomain(pathDomain); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Validate the literal path value first; the engine keys routes by
	// normalized (lowercase) domain, so resolve and tombstone by the same
	// normalized form AddRoute stored.
	domain := netutil.NormalizeHost(pathDomain)

	inc, ok := parseIncarnationQuery(r)
	if !ok {
		http.Error(w, "inc query parameter must be a single canonical positive integer", http.StatusBadRequest)
		return
	}

	removedInc, removed := s.engine.RemoveRoute(domain, inc)
	if !removed {
		// RemoveRoute is always attempted first; only on failure resolve to
		// distinguish a stale incarnation (409) from an absent route (404).
		if _, exists := s.engine.Resolve(domain); exists {
			http.Error(w, "route changed — re-confirm with a fresh incarnation", http.StatusConflict)
			return
		}
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}
	s.tracker.Unregister(domain, removedInc)
	w.WriteHeader(http.StatusNoContent)
}

// incGrammar matches the canonical positive decimal form [1-9][0-9]*: no
// sign, no leading zeros, no whitespace. Zero, negative, and malformed
// values are rejected before parsing.
var incGrammar = regexp.MustCompile(`^[1-9][0-9]*$`)

// parseIncarnationQuery extracts exactly one canonical `inc` value from the
// query string. Duplicate parameters, missing/empty values, and any
// non-canonical form (leading zeros, signs, whitespace, hex, floats,
// exponents, overflow) are rejected.
func parseIncarnationQuery(r *http.Request) (uint64, bool) {
	vals, present := r.URL.Query()["inc"]
	if !present || len(vals) != 1 {
		return 0, false
	}
	v := vals[0]
	if !incGrammar.MatchString(v) {
		return 0, false
	}
	inc, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return inc, true
}
