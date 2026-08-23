package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GhostFlying/tailpath/internal/app"
	"github.com/GhostFlying/tailpath/internal/domain"
	"github.com/GhostFlying/tailpath/internal/store"
)

type Authorizer interface {
	Authorize(context.Context, string) (string, error)
}

type Options struct {
	Authorizer            Authorizer
	WebDir                string
	Logger                *slog.Logger
	TopologyEventInterval time.Duration
}

type Server struct {
	app        *app.App
	authorizer Authorizer
	webDir     string
	logger     *slog.Logger
	mux        *http.ServeMux
	eventEvery time.Duration
}

type transportIdentityKey struct{}

func New(application *app.App, options Options) *Server {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.TopologyEventInterval <= 0 {
		options.TopologyEventInterval = 250 * time.Millisecond
	}
	server := &Server{
		app:        application,
		authorizer: options.Authorizer,
		webDir:     options.WebDir,
		logger:     options.Logger,
		mux:        http.NewServeMux(),
		eventEvery: options.TopologyEventInterval,
	}
	server.mux.HandleFunc("POST /api/v1/reports", server.submitReport)
	server.mux.HandleFunc("GET /api/v1/topology", server.getTopology)
	server.mux.HandleFunc("GET /api/v1/events", server.streamEvents)
	server.mux.HandleFunc("GET /api/v1/history/nodes", server.getHistoryNodes)
	server.mux.HandleFunc("GET /api/v1/history/edges", server.listHistoryEdges)
	server.mux.HandleFunc("GET /api/v1/history/edges/{edgeID}", server.getEdgeHistory)
	if options.FixtureMutation != nil {
		server.mux.HandleFunc("POST /api/v1/fixture/edge-update", func(response http.ResponseWriter, request *http.Request) {
			value, err := options.FixtureMutation(request.Context())
			if err != nil {
				server.logger.Error("fixture edge mutation failed", "error", err)
				writeProblem(response, http.StatusInternalServerError, "fixture mutation failed", "")
				return
			}
			writeJSON(response, http.StatusAccepted, value)
		})
	}
	server.mux.HandleFunc("GET /healthz", health)
	server.mux.HandleFunc("/", server.serveWeb)
	return server
}

func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.authorizeAPI(s.mux))
}

func (s *Server) submitReport(response http.ResponseWriter, request *http.Request) {
	transportIdentity, _ := request.Context().Value(transportIdentityKey{}).(string)
	request.Body = http.MaxBytesReader(response, request.Body, 2<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var report domain.ReportEnvelope
	if err := decoder.Decode(&report); err != nil {
		writeProblem(response, http.StatusBadRequest, "invalid report", err.Error())
		return
	}
	if err := ensureEOF(decoder); err != nil {
		writeProblem(response, http.StatusBadRequest, "invalid report", err.Error())
		return
	}
	receipt, err := s.app.Submit(request.Context(), report)
	if err != nil {
		if validation := report.Validate(); validation != nil {
			writeProblem(response, http.StatusBadRequest, "invalid report", validation.Error())
			return
		}
		s.logger.Error("report ingest failed", "transport_identity", transportIdentity, "report_id", report.ReportID, "error", err)
		writeProblem(response, http.StatusInternalServerError, "report ingest failed", "the report could not be stored")
		return
	}
	writeJSON(response, http.StatusAccepted, receipt)
}

func (s *Server) authorizeAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/api/v1/") {
			next.ServeHTTP(response, request)
			return
		}
		if s.authorizer == nil {
			writeProblem(response, http.StatusUnauthorized, "Tailnet identity is required", "API has no WhoIs authorizer")
			return
		}
		identity, err := s.authorizer.Authorize(request.Context(), request.RemoteAddr)
		if err != nil {
			writeProblem(response, http.StatusUnauthorized, "Tailnet identity is required", err.Error())
			return
		}
		ctx := context.WithValue(request.Context(), transportIdentityKey{}, identity)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func (s *Server) getTopology(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, s.app.Aggregator.Snapshot())
}

func (s *Server) getEdgeHistory(response http.ResponseWriter, request *http.Request) {
	edgeID := request.PathValue("edgeID")
	if edgeID == "" {
		writeProblem(response, http.StatusBadRequest, "edge ID is required", "")
		return
	}
	window, ok := parseHistoryWindow(response, request)
	if !ok {
		return
	}
	history, found, err := s.app.EdgeHistoryWindow(request.Context(), edgeID, window)
	if err != nil {
		if historyRequestCanceled(request, err) {
			return
		}
		s.logger.Error("edge history query failed", "edge_id", edgeID, "error", err)
		writeProblem(response, http.StatusInternalServerError, "history query failed", "")
		return
	}
	if !found {
		writeProblem(response, http.StatusNotFound, "edge history not found", "")
		return
	}
	writeJSON(response, http.StatusOK, history)
}

func (s *Server) getHistoryNodes(response http.ResponseWriter, request *http.Request) {
	window, ok := parseHistoryWindow(response, request)
	if !ok {
		return
	}
	nodes, err := s.app.HistoryNodes(request.Context(), window)
	if err != nil {
		if historyRequestCanceled(request, err) {
			return
		}
		s.logger.Error("history nodes query failed", "window", window, "error", err)
		writeProblem(response, http.StatusInternalServerError, "history query failed", "")
		return
	}
	writeJSON(response, http.StatusOK, nodes)
}

func (s *Server) listHistoryEdges(response http.ResponseWriter, request *http.Request) {
	window, ok := parseHistoryWindow(response, request)
	if !ok {
		return
	}
	query := domain.HistoryEdgeQuery{
		Window: window, NodeID: request.URL.Query().Get("nodeId"),
		Path: domain.PathKind(request.URL.Query().Get("path")), Cursor: request.URL.Query().Get("cursor"), Limit: 50,
	}
	if query.Path != "" && !validPathKind(query.Path) {
		writeProblem(response, http.StatusBadRequest, "invalid history path", "path must be direct, derp, peer_relay, or unknown")
		return
	}
	if rawLimit := request.URL.Query().Get("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > 100 {
			writeProblem(response, http.StatusBadRequest, "invalid history limit", "limit must be between 1 and 100")
			return
		}
		query.Limit = limit
	}
	page, err := s.app.HistoryEdges(request.Context(), query)
	if errors.Is(err, store.ErrInvalidHistoryCursor) {
		writeProblem(response, http.StatusBadRequest, "invalid history cursor", "")
		return
	}
	if err != nil {
		if historyRequestCanceled(request, err) {
			return
		}
		s.logger.Error("history edge list query failed", "window", window, "error", err)
		writeProblem(response, http.StatusInternalServerError, "history query failed", "")
		return
	}
	writeJSON(response, http.StatusOK, page)
}

func historyRequestCanceled(request *http.Request, err error) bool {
	return request.Context().Err() != nil &&
		(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

func parseHistoryWindow(response http.ResponseWriter, request *http.Request) (domain.HistoryWindow, bool) {
	window := domain.HistoryWindow(request.URL.Query().Get("window"))
	if !window.Valid() {
		writeProblem(response, http.StatusBadRequest, "invalid history window", "window must be 15m, 1h, 6h, 24h, or 7d")
		return "", false
	}
	return window, true
}

func validPathKind(path domain.PathKind) bool {
	switch path {
	case domain.PathDirect, domain.PathDERP, domain.PathPeerRelay, domain.PathUnknown:
		return true
	default:
		return false
	}
}

func (s *Server) streamEvents(response http.ResponseWriter, request *http.Request) {
	flusher, ok := response.(http.Flusher)
	if !ok {
		writeProblem(response, http.StatusInternalServerError, "streaming is unavailable", "")
		return
	}
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("X-Accel-Buffering", "no")
	events, unsubscribe := s.app.Aggregator.Subscribe()
	defer unsubscribe()
	invalidations := coalesceInvalidations(request.Context(), events, s.eventEvery)
	writeSSE(response, "ready", map[string]any{"generatedAt": time.Now().UTC()})
	flusher.Flush()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case _, ok := <-invalidations:
			if !ok {
				return
			}
			writeSSE(response, "topology", map[string]any{"generatedAt": time.Now().UTC()})
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(response, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func coalesceInvalidations(ctx context.Context, input <-chan struct{}, interval time.Duration) <-chan struct{} {
	output := make(chan struct{})
	go func() {
		defer close(output)
		var timer *time.Timer
		var deadline <-chan time.Time
		pending := false
		defer func() {
			if timer != nil {
				timer.Stop()
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-input:
				if !ok {
					return
				}
				pending = true
				if timer == nil {
					timer = time.NewTimer(interval)
					deadline = timer.C
				}
			case <-deadline:
				timer = nil
				deadline = nil
				if !pending {
					continue
				}
				pending = false
				select {
				case output <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return output
}

func (s *Server) serveWeb(response http.ResponseWriter, request *http.Request) {
	if s.webDir == "" {
		writeProblem(response, http.StatusNotFound, "web application is not configured", "run the Vite development server")
		return
	}
	path := strings.TrimPrefix(filepath.Clean("/"+request.URL.Path), "/")
	if path == "" || path == "." {
		path = "index.html"
	}
	fullPath := filepath.Join(s.webDir, path)
	if !withinDir(s.webDir, fullPath) {
		http.NotFound(response, request)
		return
	}
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		fullPath = filepath.Join(s.webDir, "index.html")
	}
	if extension := filepath.Ext(fullPath); extension != "" {
		response.Header().Set("Content-Type", mime.TypeByExtension(extension))
	}
	http.ServeFile(response, request, fullPath)
}

func withinDir(base, target string) bool {
	base, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(base, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(response, request)
	})
}

func health(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func writeSSE(response io.Writer, event string, value any) {
	payload, _ := json.Marshal(value)
	fmt.Fprintf(response, "event: %s\ndata: %s\n\n", event, payload)
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func writeProblem(response http.ResponseWriter, status int, title, detail string) {
	response.Header().Set("Content-Type", "application/problem+json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]any{"title": title, "status": status, "detail": detail})
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("request contains more than one JSON value")
}

type LoopbackAuthorizer struct{}

func (LoopbackAuthorizer) Authorize(_ context.Context, remoteAddr string) (string, error) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("fixture ingest only accepts loopback reporters")
	}
	return "fixture", nil
}

func HealthHandler() http.Handler {
	return http.HandlerFunc(health)
}
