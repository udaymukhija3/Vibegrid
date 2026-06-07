package vibegrid

import (
	"fmt"
	"net/http"
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
	count   uint64
	sum     float64
	buckets []uint64
}

type httpMetrics struct {
	mu       sync.Mutex
	requests map[httpMetricKey]*httpMetricSample
}

func newHTTPMetrics() *httpMetrics {
	return &httpMetrics{requests: map[httpMetricKey]*httpMetricSample{}}
}

func (metrics *httpMetrics) observe(method, route string, status int, duration time.Duration) {
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
		sample = &httpMetricSample{buckets: make([]uint64, len(httpDurationBuckets))}
		metrics.requests[key] = sample
	}
	sample.count++
	sample.sum += seconds
	for index, bucket := range httpDurationBuckets {
		if seconds <= bucket {
			sample.buckets[index]++
		}
	}
}

func (server *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	server.metrics.writePrometheus(w)
	server.writeRuntimeGauges(w)
}

// writeRuntimeGauges appends the connection-pool and puzzle-cache metrics that
// make the latency/scale machinery observable: pool saturation (wait count and
// wait time are the classic exhaustion signals) and cache hit rate. Both sources
// are optional, so this is a no-op without a database.
func (server *Server) writeRuntimeGauges(w http.ResponseWriter) {
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

	for _, key := range metrics.sortedKeys() {
		sample := metrics.sample(key)
		labels := metricLabels(key)
		fmt.Fprintf(w, "vibegrid_http_requests_total{%s} %d\n", labels, sample.count)

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

func (metrics *httpMetrics) sample(key httpMetricKey) httpMetricSample {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	source := metrics.requests[key]
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
		metrics.observe(r.Method, routeMetricLabel(r), status, time.Since(started))
	})
}

func routeMetricLabel(r *http.Request) string {
	if r.Pattern != "" {
		return strings.TrimPrefix(r.Pattern, r.Method+" ")
	}

	route := r.URL.Path
	if route == "" {
		return "/"
	}
	if strings.HasPrefix(route, "/_next/static/") {
		return "/_next/static/*"
	}
	if isAPIRoute(route) || route == "/metrics" {
		return route
	}
	if strings.Contains(strings.Trim(route, "/"), ".") {
		return "/static/*"
	}
	return "/"
}
