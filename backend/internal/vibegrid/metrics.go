package vibegrid

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var httpDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type httpMetricKey struct {
	method string
	route  string
	status string
}

type httpMetricSample struct {
	count         uint64
	sum           float64
	requestBytes  uint64
	responseBytes uint64
	buckets       []uint64
}

type httpMetrics struct {
	mu                   sync.Mutex
	requests             map[httpMetricKey]*httpMetricSample
	operations           map[operationMetricKey]*httpMetricSample
	droppedRequestSeries uint64
}

const maxHTTPMetricSeries = 256

func newHTTPMetrics() *httpMetrics {
	return &httpMetrics{
		requests:   map[httpMetricKey]*httpMetricSample{},
		operations: map[operationMetricKey]*httpMetricSample{},
	}
}

func (metrics *httpMetrics) observe(method, route string, status int, duration time.Duration, requestBytes, responseBytes int64) {
	if metrics == nil {
		return
	}

	key := httpMetricKey{
		method: method,
		route:  route,
		status: strconv.Itoa(status),
	}
	seconds := duration.Seconds()

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	sample := metrics.requests[key]
	if sample == nil {
		if len(metrics.requests) >= maxHTTPMetricSeries {
			metrics.droppedRequestSeries++
			return
		}
		sample = &httpMetricSample{buckets: make([]uint64, len(httpDurationBuckets))}
		metrics.requests[key] = sample
	}
	sample.count++
	sample.sum += seconds
	sample.requestBytes += uint64(maxInt64(requestBytes, 0))
	sample.responseBytes += uint64(maxInt64(responseBytes, 0))
	for index, bucket := range httpDurationBuckets {
		if seconds <= bucket {
			sample.buckets[index]++
		}
	}
}

type operationMetricKey struct {
	component string
	operation string
	status    string
}

func (metrics *httpMetrics) observeOperation(component, operation string, status string, duration time.Duration) {
	if metrics == nil {
		return
	}

	key := operationMetricKey{component: component, operation: operation, status: status}
	seconds := duration.Seconds()

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	sample := metrics.operations[key]
	if sample == nil {
		sample = &httpMetricSample{buckets: make([]uint64, len(httpDurationBuckets))}
		metrics.operations[key] = sample
	}
	sample.count++
	sample.sum += seconds
	for index, bucket := range httpDurationBuckets {
		if seconds <= bucket {
			sample.buckets[index]++
		}
	}
}

func (server *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	server.metrics.writePrometheus(w)
	server.writeRuntimeGauges(w, r.Context())
	writeProcessGauges(w)
}

// writeRuntimeGauges appends the connection-pool and puzzle-cache metrics that
// make the latency/scale machinery observable: pool saturation (wait count and
// wait time are the classic exhaustion signals) and cache hit rate. Both sources
// are optional, so this is a no-op without a database.
func (server *Server) writeRuntimeGauges(w http.ResponseWriter, ctx context.Context) {
	if server.dbStats != nil {
		stats := server.dbStats()
		writeGauge(w, "vibegrid_db_open_connections", "Open Postgres connections (in use plus idle).", float64(stats.OpenConnections))
		writeGauge(w, "vibegrid_db_in_use_connections", "Postgres connections currently in use.", float64(stats.InUse))
		writeGauge(w, "vibegrid_db_idle_connections", "Idle Postgres connections in the pool.", float64(stats.Idle))
		writeCounter(w, "vibegrid_db_wait_count_total", "Total times a request waited for a free connection.", float64(stats.WaitCount))
		writeCounter(w, "vibegrid_db_wait_seconds_total", "Total time requests spent waiting for a connection.", stats.WaitDuration.Seconds())
	}
	if server.puzzleCacheStats != nil {
		cache := server.puzzleCacheStats()
		writeCounter(w, "vibegrid_puzzle_cache_hits_total", "Puzzle content cache hits.", float64(cache.Hits))
		writeCounter(w, "vibegrid_puzzle_cache_misses_total", "Puzzle content cache misses.", float64(cache.Misses))
		writeCounter(w, "vibegrid_puzzle_cache_evictions_total", "Puzzle content cache evictions.", float64(cache.Evictions))
		writeGauge(w, "vibegrid_puzzle_cache_entries", "Puzzles currently held in the content cache.", float64(cache.Entries))
	}
	if server.outboxStats != nil {
		statsCtx, cancel := context.WithTimeout(ctx, time.Second)
		stats := server.outboxStats(statsCtx)
		cancel()
		writeGauge(w, "vibegrid_notification_outbox_pending", "New notification events waiting for delivery.", float64(stats.Pending))
		writeGauge(w, "vibegrid_notification_outbox_retrying", "Notification events waiting after a failed delivery.", float64(stats.Retrying))
		writeGauge(w, "vibegrid_notification_outbox_dead", "Notification events that exhausted delivery attempts.", float64(stats.Dead))
		writeGauge(w, "vibegrid_notification_outbox_oldest_pending_seconds", "Age of the oldest undelivered notification event.", stats.OldestPendingSeconds)
	}
}

func writeProcessGauges(w http.ResponseWriter) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	writeGauge(w, "vibegrid_process_goroutines", "Current goroutine count.", float64(runtime.NumGoroutine()))
	writeGauge(w, "vibegrid_process_heap_alloc_bytes", "Bytes allocated and still in use on the heap.", float64(memory.HeapAlloc))
	writeGauge(w, "vibegrid_process_heap_inuse_bytes", "Heap bytes currently in use by spans.", float64(memory.HeapInuse))
	writeGauge(w, "vibegrid_process_stack_inuse_bytes", "Stack bytes currently in use.", float64(memory.StackInuse))
	writeGauge(w, "vibegrid_process_sys_bytes", "Total bytes obtained from the OS by the Go runtime.", float64(memory.Sys))
	writeCounter(w, "vibegrid_process_gc_cycles_total", "Completed Go GC cycles.", float64(memory.NumGC))
}

func writeGauge(w http.ResponseWriter, name, help string, value float64) {
	writeMetric(w, name, help, "gauge", value)
}

func writeCounter(w http.ResponseWriter, name, help string, value float64) {
	writeMetric(w, name, help, "counter", value)
}

func writeMetric(w http.ResponseWriter, name, help, metricType string, value float64) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s %s\n", name, metricType)
	fmt.Fprintf(w, "%s %s\n", name, strconv.FormatFloat(value, 'f', -1, 64))
}

func (metrics *httpMetrics) writePrometheus(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	fmt.Fprintln(w, "# HELP vibegrid_up Whether the VibeGrid process is serving requests.")
	fmt.Fprintln(w, "# TYPE vibegrid_up gauge")
	fmt.Fprintln(w, "vibegrid_up 1")
	fmt.Fprintln(w, "# HELP vibegrid_http_requests_total HTTP requests served by the app.")
	fmt.Fprintln(w, "# TYPE vibegrid_http_requests_total counter")
	fmt.Fprintln(w, "# HELP vibegrid_http_request_duration_seconds HTTP request latency.")
	fmt.Fprintln(w, "# TYPE vibegrid_http_request_duration_seconds histogram")
	fmt.Fprintln(w, "# HELP vibegrid_http_request_bytes_total HTTP request body bytes received by the app.")
	fmt.Fprintln(w, "# TYPE vibegrid_http_request_bytes_total counter")
	fmt.Fprintln(w, "# HELP vibegrid_http_response_bytes_total HTTP response body bytes written by the app.")
	fmt.Fprintln(w, "# TYPE vibegrid_http_response_bytes_total counter")
	fmt.Fprintln(w, "# HELP vibegrid_http_metric_series_dropped_total HTTP metric observations dropped because the series cap was reached.")
	fmt.Fprintln(w, "# TYPE vibegrid_http_metric_series_dropped_total counter")
	fmt.Fprintf(w, "vibegrid_http_metric_series_dropped_total %d\n", metrics.droppedSeries())
	fmt.Fprintln(w, "# HELP vibegrid_store_operations_total Storage interface operations by component, operation, and status.")
	fmt.Fprintln(w, "# TYPE vibegrid_store_operations_total counter")
	fmt.Fprintln(w, "# HELP vibegrid_store_operation_duration_seconds Storage interface operation latency.")
	fmt.Fprintln(w, "# TYPE vibegrid_store_operation_duration_seconds histogram")

	for _, key := range metrics.sortedKeys() {
		sample := metrics.sample(key)
		labels := metricLabels(key)
		fmt.Fprintf(w, "vibegrid_http_requests_total{%s} %d\n", labels, sample.count)
		fmt.Fprintf(w, "vibegrid_http_request_bytes_total{%s} %d\n", labels, sample.requestBytes)
		fmt.Fprintf(w, "vibegrid_http_response_bytes_total{%s} %d\n", labels, sample.responseBytes)

		var cumulative uint64
		for index, bucket := range httpDurationBuckets {
			cumulative = sample.buckets[index]
			fmt.Fprintf(
				w,
				"vibegrid_http_request_duration_seconds_bucket{%s,le=%q} %d\n",
				labels,
				strconv.FormatFloat(bucket, 'f', -1, 64),
				cumulative,
			)
		}
		fmt.Fprintf(w, "vibegrid_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, sample.count)
		fmt.Fprintf(w, "vibegrid_http_request_duration_seconds_sum{%s} %.6f\n", labels, sample.sum)
		fmt.Fprintf(w, "vibegrid_http_request_duration_seconds_count{%s} %d\n", labels, sample.count)
	}

	for _, key := range metrics.sortedOperationKeys() {
		sample := metrics.operationSample(key)
		labels := operationMetricLabels(key)
		fmt.Fprintf(w, "vibegrid_store_operations_total{%s} %d\n", labels, sample.count)

		var cumulative uint64
		for index, bucket := range httpDurationBuckets {
			cumulative = sample.buckets[index]
			fmt.Fprintf(
				w,
				"vibegrid_store_operation_duration_seconds_bucket{%s,le=%q} %d\n",
				labels,
				strconv.FormatFloat(bucket, 'f', -1, 64),
				cumulative,
			)
		}
		fmt.Fprintf(w, "vibegrid_store_operation_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, sample.count)
		fmt.Fprintf(w, "vibegrid_store_operation_duration_seconds_sum{%s} %.6f\n", labels, sample.sum)
		fmt.Fprintf(w, "vibegrid_store_operation_duration_seconds_count{%s} %d\n", labels, sample.count)
	}
}

func (metrics *httpMetrics) droppedSeries() uint64 {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	return metrics.droppedRequestSeries
}

func (metrics *httpMetrics) sortedKeys() []httpMetricKey {
	if metrics == nil {
		return nil
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	keys := make([]httpMetricKey, 0, len(metrics.requests))
	for key := range metrics.requests {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		a := keys[left]
		b := keys[right]
		if a.route != b.route {
			return a.route < b.route
		}
		if a.method != b.method {
			return a.method < b.method
		}
		return a.status < b.status
	})
	return keys
}

func (metrics *httpMetrics) sortedOperationKeys() []operationMetricKey {
	if metrics == nil {
		return nil
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	keys := make([]operationMetricKey, 0, len(metrics.operations))
	for key := range metrics.operations {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		a := keys[left]
		b := keys[right]
		if a.component != b.component {
			return a.component < b.component
		}
		if a.operation != b.operation {
			return a.operation < b.operation
		}
		return a.status < b.status
	})
	return keys
}

func (metrics *httpMetrics) sample(key httpMetricKey) httpMetricSample {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	source := metrics.requests[key]
	if source == nil {
		return httpMetricSample{buckets: make([]uint64, len(httpDurationBuckets))}
	}
	return httpMetricSample{
		count:         source.count,
		sum:           source.sum,
		requestBytes:  source.requestBytes,
		responseBytes: source.responseBytes,
		buckets:       append([]uint64(nil), source.buckets...),
	}
}

func (metrics *httpMetrics) operationSample(key operationMetricKey) httpMetricSample {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	source := metrics.operations[key]
	if source == nil {
		return httpMetricSample{buckets: make([]uint64, len(httpDurationBuckets))}
	}
	return httpMetricSample{
		count:   source.count,
		sum:     source.sum,
		buckets: append([]uint64(nil), source.buckets...),
	}
}

func metricLabels(key httpMetricKey) string {
	return fmt.Sprintf(
		`method="%s",route="%s",status="%s"`,
		escapeMetricLabel(key.method),
		escapeMetricLabel(key.route),
		escapeMetricLabel(key.status),
	)
}

func operationMetricLabels(key operationMetricKey) string {
	return fmt.Sprintf(
		`component="%s",operation="%s",status="%s"`,
		escapeMetricLabel(key.component),
		escapeMetricLabel(key.operation),
		escapeMetricLabel(key.status),
	)
}

func escapeMetricLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func withRequestMetrics(next http.Handler, metrics *httpMetrics) http.Handler {
	if metrics == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		metrics.observe(r.Method, routeMetricLabel(r), status, time.Since(started), requestBodyBytes(r), recorder.bytesWritten)
	})
}

func requestBodyBytes(r *http.Request) int64 {
	if r.ContentLength <= 0 {
		return 0
	}
	return r.ContentLength
}

func routeMetricLabel(r *http.Request) string {
	route := r.URL.Path
	if known := knownRouteMetricLabel(route); known != "" {
		return known
	}
	if route == "" {
		return "/"
	}
	if strings.HasPrefix(route, "/_next/static/") {
		return "/_next/static/*"
	}
	if isAPIRoute(route) {
		if route == "/metrics" {
			return "/metrics"
		}
		return "/api/*"
	}
	if strings.Contains(strings.Trim(route, "/"), ".") {
		return "/static/*"
	}
	return "/"
}

// The outer middleware intentionally does not depend on Request.Pattern: the
// standard mux may update a request copy while the metrics wrapper still sees
// the original. This explicit allowlist keeps labels stable even for malformed
// paths and unknown HTTP methods.
func knownRouteMetricLabel(route string) string {
	switch route {
	case "/healthz", "/readyz", "/metrics", "/api/puzzles/today", "/api/vibes/today", "/api/puzzles", "/api/puzzle-templates", "/api/public-config", "/api/session", "/api/streak", "/api/guesses", "/api/community/puzzles", "/api/crews", "/api/reports", "/api/appeals", "/api/client-errors", "/api/admin/session", "/api/admin/queue-health", "/api/admin/vibe-boards", "/api/admin/puzzles", "/api/admin/moderation/reports", "/api/admin/moderation/appeals", "/api/admin/moderation/audit":
		return route
	}
	if strings.HasPrefix(route, "/api/attempts/") {
		return "/api/attempts/{id}"
	}
	if strings.HasPrefix(route, "/api/og/puzzles/") {
		return "/api/og/puzzles/{id}"
	}
	if strings.HasPrefix(route, "/api/puzzles/") {
		switch {
		case strings.HasSuffix(route, "/stats"):
			return "/api/puzzles/{id}/stats"
		case strings.HasSuffix(route, "/vibes"):
			return "/api/puzzles/{id}/vibes"
		case strings.HasSuffix(route, "/easy-hint"):
			return "/api/puzzles/{id}/easy-hint"
		default:
			return "/api/puzzles/{id}"
		}
	}
	if strings.HasPrefix(route, "/api/community/puzzles/") {
		switch {
		case strings.HasSuffix(route, "/claim"):
			return "/api/community/puzzles/{id}/claim"
		case strings.HasSuffix(route, "/withdraw"):
			return "/api/community/puzzles/{id}/withdraw"
		}
	}
	if strings.HasPrefix(route, "/api/crews/") {
		switch {
		case strings.HasSuffix(route, "/daily"):
			return "/api/crews/{id}/daily"
		case strings.HasSuffix(route, "/submissions"):
			return "/api/crews/{id}/submissions"
		case strings.HasSuffix(route, "/votes"):
			return "/api/crews/{id}/votes"
		case strings.HasSuffix(route, "/join"):
			return "/api/crews/{id}/join"
		case strings.HasSuffix(route, "/rotate"):
			return "/api/crews/{id}/rotate"
		case strings.HasSuffix(route, "/leave"):
			return "/api/crews/{id}/leave"
		case strings.Contains(route, "/members/") && strings.HasSuffix(route, "/remove"):
			return "/api/crews/{id}/members/{memberId}/remove"
		default:
			return "/api/crews/{id}"
		}
	}
	if strings.HasPrefix(route, "/api/admin/puzzles/") {
		switch {
		case strings.HasSuffix(route, "/publish"):
			return "/api/admin/puzzles/{id}/publish"
		case strings.HasSuffix(route, "/approve"):
			return "/api/admin/puzzles/{id}/approve"
		case strings.HasSuffix(route, "/archive"):
			return "/api/admin/puzzles/{id}/archive"
		case strings.HasSuffix(route, "/reinstate"):
			return "/api/admin/puzzles/{id}/reinstate"
		case strings.HasSuffix(route, "/preview"):
			return "/api/admin/puzzles/{id}/preview"
		case strings.HasSuffix(route, "/analytics"):
			return "/api/admin/puzzles/{id}/analytics"
		}
	}
	if strings.HasPrefix(route, "/api/admin/moderation/reports/") {
		return "/api/admin/moderation/reports/{id}/resolve"
	}
	if strings.HasPrefix(route, "/api/admin/moderation/appeals/") {
		return "/api/admin/moderation/appeals/{id}/resolve"
	}
	return ""
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
