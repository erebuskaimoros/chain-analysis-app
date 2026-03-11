package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const requestIDHeader = "X-Request-ID"

type ctxKey string

const requestIDContextKey ctxKey = "request_id"
const runLogCaptureContextKey ctxKey = "run_log_capture"

var requestIDFallbackCounter uint64

type runLogCapture struct {
	mu    sync.Mutex
	lines []string
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)
		r = r.WithContext(ctx)

		wrapped := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		wrapped.Header().Set(requestIDHeader, requestID)

		started := time.Now()
		logInfo(ctx, "http_request_started", map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
		})

		next.ServeHTTP(wrapped, r)

		logInfo(ctx, "http_request_completed", map[string]any{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      wrapped.status,
			"bytes":       wrapped.bytes,
			"duration_ms": time.Since(started).Milliseconds(),
		})
	})
}

func WithRequestLogging(next http.Handler) http.Handler {
	return withRequestLogging(next)
}

func WithRequestLoggingFunc(next http.HandlerFunc) http.HandlerFunc {
	return WithRequestLogging(next).ServeHTTP
}

func LogError(ctx context.Context, event string, err error, fields map[string]any) {
	logError(ctx, event, err, fields)
}

func (a *App) withRequestLoggingFunc(next http.HandlerFunc) http.HandlerFunc {
	return a.withRequestLogging(next).ServeHTTP
}

func (a *App) withRequestLogging(next http.Handler) http.Handler {
	return withRequestLogging(next)
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, ok := ctx.Value(requestIDContextKey).(string)
	if !ok {
		return ""
	}
	return value
}

func withRunLogCapture(ctx context.Context) (context.Context, *runLogCapture) {
	capture := &runLogCapture{}
	return context.WithValue(ctx, runLogCaptureContextKey, capture), capture
}

func runLogCaptureFromContext(ctx context.Context) *runLogCapture {
	if ctx == nil {
		return nil
	}
	capture, _ := ctx.Value(runLogCaptureContextKey).(*runLogCapture)
	return capture
}

func (c *runLogCapture) append(line string) {
	if c == nil || strings.TrimSpace(line) == "" {
		return
	}
	c.mu.Lock()
	c.lines = append(c.lines, line)
	c.mu.Unlock()
}

func (c *runLogCapture) snapshot() []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.lines))
	copy(out, c.lines)
	return out
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "req-" + hex.EncodeToString(b[:])
	}
	n := atomic.AddUint64(&requestIDFallbackCounter, 1)
	return "req-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10) + "-" + fmt.Sprintf("%06d", n)
}

func logInfo(ctx context.Context, event string, fields map[string]any) {
	logStructured(ctx, "info", event, fields)
}

func logError(ctx context.Context, event string, err error, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	logStructured(ctx, "error", event, fields)
}

func logStructured(ctx context.Context, level string, event string, fields map[string]any) {
	payload := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": strings.ToLower(strings.TrimSpace(level)),
		"event": strings.TrimSpace(event),
	}
	if requestID := requestIDFromContext(ctx); requestID != "" {
		payload["request_id"] = requestID
	}
	for k, v := range fields {
		if _, exists := payload[k]; !exists {
			payload[k] = v
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		log.Printf(`{"ts":"%s","level":"error","event":"log_marshal_failed","error":%q}`,
			time.Now().UTC().Format(time.RFC3339Nano),
			err.Error(),
		)
		return
	}
	if capture := runLogCaptureFromContext(ctx); capture != nil {
		capture.append(string(raw))
	}
	log.Print(string(raw))
}
