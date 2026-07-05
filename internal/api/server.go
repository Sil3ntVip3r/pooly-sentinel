package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sil3ntVip3r/pooly-sentinel/internal/agent"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/redaction"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/reports"
	"github.com/Sil3ntVip3r/pooly-sentinel/internal/storage"
)

const defaultLimit = 100
const maxLimit = 500

type Store interface {
	Ping(ctx context.Context) error
	SchemaVersion(ctx context.Context) (int, error)
	IncidentStatusCounts(ctx context.Context) (map[string]int64, error)
	NotificationDeliveryStatusCounts(ctx context.Context) (map[string]int64, error)
	ListRecentIncidents(ctx context.Context, limit int) ([]storage.IncidentRecord, error)
	GetIncident(ctx context.Context, id string) (storage.IncidentRecord, error)
	ListRecentNotificationDeliveries(ctx context.Context, incidentID string, limit int) ([]storage.NotificationDeliveryRecord, error)
}

type Options struct {
	Enabled          bool
	Listen           string
	AllowNonLoopback bool
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	ShutdownTimeout  time.Duration
	Store            Store
	Reports          reports.Options
	SchedulerStatus  func() agent.SchedulerStatus
	Listener         net.Listener
	Now              func() time.Time
}

type Server struct {
	opts     Options
	server   *http.Server
	listener net.Listener
	ready    atomic.Bool
	mu       sync.Mutex
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewServer(opts Options) (*Server, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.ReadTimeout <= 0 {
		opts.ReadTimeout = 5 * time.Second
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = 10 * time.Second
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 10 * time.Second
	}
	if opts.Listen == "" {
		opts.Listen = "127.0.0.1:9587"
	}
	if opts.Enabled {
		if err := validateListen(opts.Listen, opts.AllowNonLoopback); err != nil {
			return nil, err
		}
	}
	s := &Server{opts: opts}
	s.server = &http.Server{
		Handler:      s.Handler(),
		ReadTimeout:  opts.ReadTimeout,
		WriteTimeout: opts.WriteTimeout,
	}
	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	if s == nil || !s.opts.Enabled {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return nil
	}
	listener := s.opts.Listener
	if listener == nil {
		var lc net.ListenConfig
		var err error
		listener, err = lc.Listen(ctx, "tcp", s.opts.Listen)
		if err != nil {
			return fmt.Errorf("api listen: %w", redaction.Error(err))
		}
	}
	s.listener = listener
	go func() {
		err := s.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			// The run lifecycle owns logging; endpoint behavior stays read-only.
		}
	}()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || !s.opts.Enabled {
		return nil
	}
	s.SetReady(false)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	shutdownCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok && s.opts.ShutdownTimeout > 0 {
		shutdownCtx, cancel = context.WithTimeout(ctx, s.opts.ShutdownTimeout)
	}
	defer cancel()
	err := s.server.Shutdown(shutdownCtx)
	s.listener = nil
	if err != nil {
		return fmt.Errorf("api shutdown: %w", redaction.Error(err))
	}
	return nil
}

func (s *Server) SetReady(ready bool) {
	if s != nil {
		s.ready.Store(ready)
	}
}

func (s *Server) Ready() bool {
	return s != nil && s.ready.Load()
}

func (s *Server) Addr() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.handle)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	switch {
	case r.URL.Path == "/healthz":
		s.handleHealth(w)
	case r.URL.Path == "/readyz":
		s.handleReady(w)
	case r.URL.Path == "/status" || r.URL.Path == "/metrics/status":
		s.handleStatus(w, r)
	case r.URL.Path == "/incidents":
		s.handleIncidents(w, r)
	case strings.HasPrefix(r.URL.Path, "/incidents/"):
		s.handleIncident(w, r)
	case r.URL.Path == "/notifications/deliveries":
		s.handleDeliveries(w, r)
	case r.URL.Path == "/reports/summary":
		s.handleReport(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) handleHealth(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   s.now(),
	})
}

func (s *Server) handleReady(w http.ResponseWriter) {
	ready := s.Ready()
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"ready": ready,
		"time":  s.now(),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.status(r.Context())
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleIncidents(w http.ResponseWriter, r *http.Request) {
	store := s.opts.Store
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	limit, ok := parseLimit(w, r.URL.Query())
	if !ok {
		return
	}
	records, err := store.ListRecentIncidents(r.Context(), limit)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	output := make([]IncidentResponse, 0, len(records))
	for _, record := range records {
		output = append(output, safeIncident(record))
	}
	writeJSON(w, http.StatusOK, output)
}

func (s *Server) handleIncident(w http.ResponseWriter, r *http.Request) {
	store := s.opts.Store
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/incidents/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	unescaped, err := url.PathUnescape(id)
	if err != nil || strings.TrimSpace(unescaped) == "" {
		writeError(w, http.StatusBadRequest, "invalid incident id")
		return
	}
	record, err := store.GetIncident(r.Context(), unescaped)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, safeIncident(record))
}

func (s *Server) handleDeliveries(w http.ResponseWriter, r *http.Request) {
	store := s.opts.Store
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	limit, ok := parseLimit(w, r.URL.Query())
	if !ok {
		return
	}
	incidentID := r.URL.Query().Get("incident_id")
	records, err := store.ListRecentNotificationDeliveries(r.Context(), incidentID, limit)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	output := make([]DeliveryResponse, 0, len(records))
	for _, record := range records {
		output = append(output, safeDelivery(record))
	}
	writeJSON(w, http.StatusOK, output)
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if !s.opts.Reports.Enabled {
		writeError(w, http.StatusNotFound, "reports disabled")
		return
	}
	opts := s.opts.Reports
	opts.Now = s.opts.Now
	summary, err := reports.Generate(r.Context(), reportStore{s.opts.Store}, opts)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) status(ctx context.Context) StatusResponse {
	response := StatusResponse{
		ServiceStatus:    "running",
		CurrentTime:      s.now(),
		Readiness:        s.Ready(),
		IncidentCounts:   map[string]int64{},
		DeliveryCounts:   map[string]int64{},
		StorageAvailable: false,
	}
	if s.opts.SchedulerStatus != nil {
		response.Scheduler = agent.SafeSchedulerStatus(s.opts.SchedulerStatus())
	}
	store := s.opts.Store
	if store == nil {
		response.ServiceStatus = "degraded"
		response.ErrorClass = "storage"
		response.ErrorSummary = "storage is unavailable"
		return response
	}
	if err := store.Ping(ctx); err != nil {
		response.ServiceStatus = "degraded"
		response.ErrorClass = "storage"
		response.ErrorSummary = redaction.Redact(err.Error())
		return response
	}
	response.StorageAvailable = true
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		response.ServiceStatus = "degraded"
		response.ErrorClass = "storage"
		response.ErrorSummary = redaction.Redact(err.Error())
		return response
	}
	response.SchemaVersion = version
	incidentCounts, err := store.IncidentStatusCounts(ctx)
	if err != nil {
		response.ServiceStatus = "degraded"
		response.ErrorClass = "storage"
		response.ErrorSummary = redaction.Redact(err.Error())
		return response
	}
	response.IncidentCounts = redactCountMap(incidentCounts)
	response.OpenIncidentCount = incidentCounts["open"]
	response.ResolvedIncidentCount = incidentCounts["resolved"]
	deliveryCounts, err := store.NotificationDeliveryStatusCounts(ctx)
	if err != nil {
		response.ServiceStatus = "degraded"
		response.ErrorClass = "storage"
		response.ErrorSummary = redaction.Redact(err.Error())
		return response
	}
	response.DeliveryCounts = redactCountMap(deliveryCounts)
	for _, count := range deliveryCounts {
		response.NotificationDeliveryCount += count
	}
	return response
}

func (s *Server) now() time.Time {
	if s.opts.Now == nil {
		return time.Now().UTC()
	}
	return s.opts.Now().UTC()
}

func validateListen(listen string, allowNonLoopback bool) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("api listen address must be host:port")
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("api listen host must be localhost or an IP address")
	}
	if !ip.IsLoopback() && !allowNonLoopback {
		return fmt.Errorf("api listen address must be loopback unless explicitly allowed")
	}
	return nil
}

func parseLimit(w http.ResponseWriter, values url.Values) (int, bool) {
	raw := values.Get("limit")
	if raw == "" {
		return defaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxLimit {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return 0, false
	}
	return limit, true
}

func writeStorageError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "storage error")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: redaction.Redact(message)})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	_ = encoder.Encode(value)
}

func redactCountMap(input map[string]int64) map[string]int64 {
	output := make(map[string]int64, len(input))
	for key, value := range input {
		output[redaction.Redact(key)] = value
	}
	return output
}

func safeEvidencePath(path string) string {
	if path == "" || redaction.Redact(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return ""
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.Contains(clean, "..") {
		return ""
	}
	return clean
}

type reportStore struct {
	store Store
}

func (s reportStore) Ping(ctx context.Context) error {
	if s.store == nil {
		return storage.ErrClosed
	}
	return s.store.Ping(ctx)
}

func (s reportStore) SchemaVersion(ctx context.Context) (int, error) {
	return s.store.SchemaVersion(ctx)
}

func (s reportStore) IncidentStatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.store.IncidentStatusCounts(ctx)
}

func (s reportStore) NotificationDeliveryStatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.store.NotificationDeliveryStatusCounts(ctx)
}

func (s reportStore) ListRecentIncidents(ctx context.Context, limit int) ([]storage.IncidentRecord, error) {
	return s.store.ListRecentIncidents(ctx, limit)
}
