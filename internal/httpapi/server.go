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
	Authorizer             Authorizer
	WebDir                 string
	Logger                 *slog.Logger
	DeviceDirectoryEnabled bool
	TopologyEventInterval  time.Duration
	FixtureMutation        func(context.Context) (any, error)
	FixtureLifecycle       func(context.Context) (any, error)
}

type Server struct {
	app                    *app.App
	authorizer             Authorizer
	webDir                 string
	logger                 *slog.Logger
	mux                    *http.ServeMux
	eventEvery             time.Duration
	deviceDirectoryEnabled bool
}

type deviceDirectoryResponse struct {
	Sync    deviceDirectorySyncResponse `json:"sync"`
	Devices []deviceDirectoryNode       `json:"devices"`
}

type deviceDirectorySyncResponse struct {
	Status              domain.DirectorySyncStatus `json:"status"`
	LastAttemptAt       *time.Time                 `json:"lastAttemptAt,omitempty"`
	LastSuccessAt       *time.Time                 `json:"lastSuccessAt,omitempty"`
	NextRetryAt         *time.Time                 `json:"nextRetryAt,omitempty"`
	ErrorCode           domain.DirectoryErrorCode  `json:"errorCode,omitempty"`
	InvalidAddressCount int                        `json:"invalidAddressCount"`
}

type deviceDirectoryNode struct {
	ID                 string                    `json:"id"`
	StableNodeID       string                    `json:"stableNodeId"`
	DNSName            string                    `json:"dnsName,omitempty"`
	Hostname           string                    `json:"hostname,omitempty"`
	Platform           string                    `json:"platform,omitempty"`
	TailscaleIPs       []string                  `json:"tailscaleIps"`
	Tags               []string                  `json:"tags"`
	ConnectedToControl bool                      `json:"connectedToControl"`
	LastSeen           *time.Time                `json:"lastSeen,omitempty"`
	CollectedAt        time.Time                 `json:"collectedAt"`
	Runtime            *deviceDirectoryRuntime   `json:"runtime,omitempty"`
	IdentityStatus     domain.IdentityStatus     `json:"identityStatus"`
	Conflicts          []domain.MetadataConflict `json:"conflicts"`
}

type deviceDirectoryRuntime struct {
	DNSName        string    `json:"dnsName,omitempty"`
	Hostname       string    `json:"hostname,omitempty"`
	Platform       string    `json:"platform,omitempty"`
	TailscaleIPs   []string  `json:"tailscaleIps"`
	Observable     bool      `json:"observable"`
	Online         bool      `json:"online"`
	LastEvidenceAt time.Time `json:"lastEvidenceAt"`
	CollectedAt    time.Time `json:"collectedAt"`
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
		app:                    application,
		authorizer:             options.Authorizer,
		webDir:                 options.WebDir,
		logger:                 options.Logger,
		mux:                    http.NewServeMux(),
		eventEvery:             options.TopologyEventInterval,
		deviceDirectoryEnabled: options.DeviceDirectoryEnabled,
	}
	server.mux.HandleFunc("GET /api/v1/capabilities", server.getCapabilities)
	server.mux.HandleFunc("POST /api/v1/reports", server.submitReport)
	server.mux.HandleFunc("GET /api/v1/topology", server.getTopology)
	server.mux.HandleFunc("GET /api/v1/devices", server.getDevices)
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
	if options.FixtureLifecycle != nil {
		server.mux.HandleFunc("POST /api/v1/fixture/observer-lifecycle", func(response http.ResponseWriter, request *http.Request) {
			value, err := options.FixtureLifecycle(request.Context())
			if err != nil {
				server.logger.Error("fixture observer lifecycle failed", "error", err)
				writeProblem(response, http.StatusInternalServerError, "fixture lifecycle failed", "")
				return
			}
			writeJSON(response, http.StatusAccepted, value)
		})
	}
	server.mux.HandleFunc("GET /healthz", health)
	server.mux.HandleFunc("/", server.serveWeb)
	return server
}

func (s *Server) getCapabilities(response http.ResponseWriter, _ *http.Request) {
	capabilities := domain.CurrentServerCapabilities()
	if s.deviceDirectoryEnabled {
		capabilities.Features = append(capabilities.Features, domain.FeatureDeviceDirectory)
	}
	writeJSON(response, http.StatusOK, capabilities)
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
		response.Header().Set("Cache-Control", "no-store")
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

func (s *Server) getDevices(response http.ResponseWriter, _ *http.Request) {
	directory := s.app.DeviceDirectory()
	result := deviceDirectoryResponse{
		Sync: deviceDirectorySyncResponse{
			Status: directory.Sync.Status, LastAttemptAt: directory.Sync.LastAttemptAt,
			LastSuccessAt: directory.Sync.LastSuccessAt, NextRetryAt: directory.Sync.NextRetryAt,
			ErrorCode: directory.Sync.ErrorCode, InvalidAddressCount: directory.Sync.InvalidAddressCount,
		},
		Devices: make([]deviceDirectoryNode, 0, len(directory.Devices)),
	}
	for _, entry := range directory.Devices {
		device := entry.Device
		item := deviceDirectoryNode{
			ID: entry.ID, StableNodeID: device.StableNodeID, DNSName: device.DNSName, Hostname: device.Hostname,
			Platform: device.OS, TailscaleIPs: append([]string{}, device.TailscaleIPs...),
			Tags: append([]string{}, device.Tags...), ConnectedToControl: device.ConnectedToControl,
			LastSeen: cloneTimePointer(device.LastSeen), CollectedAt: entry.CollectedAt,
			IdentityStatus: entry.IdentityStatus, Conflicts: append([]domain.MetadataConflict{}, entry.Conflicts...),
		}
		if entry.Runtime != nil {
			item.Runtime = &deviceDirectoryRuntime{
				DNSName: entry.Runtime.Identity.DNSName, Hostname: entry.Runtime.Identity.Hostname,
				Platform:     entry.Runtime.Identity.OS,
				TailscaleIPs: append([]string{}, entry.Runtime.Identity.TailscaleIPs...),
				Observable:   entry.Runtime.Observable, Online: entry.Runtime.Online,
				LastEvidenceAt: entry.Runtime.LastEvidenceAt, CollectedAt: entry.Runtime.CollectedAt,
			}
		}
		result.Devices = append(result.Devices, item)
	}
	writeJSON(response, http.StatusOK, result)
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := *value
	return &result
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
	includeSystemTelemetry, ok := parseIncludeSystemTelemetry(response, request)
	if !ok {
		return
	}
	history, found, err := s.app.EdgeHistoryWindow(request.Context(), edgeID, window, includeSystemTelemetry)
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
	normalizeHistoryCollections(&history)
	writeJSON(response, http.StatusOK, history)
}

func normalizeHistoryCollections(history *domain.EdgeHistory) {
	if history.Traffic == nil {
		history.Traffic = []domain.TrafficBucket{}
	}
	if history.PathEvents == nil {
		history.PathEvents = []domain.PathEvent{}
	}
	if history.RelatedNodes == nil {
		history.RelatedNodes = []domain.HistoryNodeReference{}
	}
	if history.PathAnchor != nil && history.PathAnchor.Observations == nil {
		history.PathAnchor.Observations = []domain.ObservationProvenance{}
	}
	if history.PathAnchor != nil && history.PathAnchor.Conflicts == nil {
		history.PathAnchor.Conflicts = []domain.PathObservation{}
	}
	for index := range history.PathEvents {
		if history.PathEvents[index].Observations == nil {
			history.PathEvents[index].Observations = []domain.ObservationProvenance{}
		}
		if history.PathEvents[index].Conflicts == nil {
			history.PathEvents[index].Conflicts = []domain.PathObservation{}
		}
	}
}

func (s *Server) getHistoryNodes(response http.ResponseWriter, request *http.Request) {
	window, ok := parseHistoryWindow(response, request)
	if !ok {
		return
	}
	includeSystemTelemetry, ok := parseIncludeSystemTelemetry(response, request)
	if !ok {
		return
	}
	nodes, err := s.app.HistoryNodes(request.Context(), window, includeSystemTelemetry)
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
	includeSystemTelemetry, ok := parseIncludeSystemTelemetry(response, request)
	if !ok {
		return
	}
	query := domain.HistoryEdgeQuery{
		Window: window, NodeID: request.URL.Query().Get("nodeId"),
		Path: domain.PathKind(request.URL.Query().Get("path")), Cursor: request.URL.Query().Get("cursor"), Limit: 50,
		IncludeSystemTelemetry: includeSystemTelemetry,
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

func parseIncludeSystemTelemetry(response http.ResponseWriter, request *http.Request) (bool, bool) {
	switch request.URL.Query().Get("includeSystemTelemetry") {
	case "", "false":
		return false, true
	case "true":
		return true, true
	default:
		writeProblem(response, http.StatusBadRequest, "invalid system telemetry option", "includeSystemTelemetry must be true or false")
		return false, false
	}
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
				if timer == nil {
					select {
					case output <- struct{}{}:
					case <-ctx.Done():
						return
					}
					timer = time.NewTimer(interval)
					deadline = timer.C
				} else {
					pending = true
				}
			case <-deadline:
				if !pending {
					timer = nil
					deadline = nil
					continue
				}
				pending = false
				select {
				case output <- struct{}{}:
				case <-ctx.Done():
					return
				}
				timer.Reset(interval)
				deadline = timer.C
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
