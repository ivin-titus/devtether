package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ivin-titus/devtether/internal/logger"
)

// loggingResponseWriter wraps an http.ResponseWriter to capture the HTTP
// status code and response size. It strictly adheres to the Single
// Responsibility Principle (SRP) to remain highly performant.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	wroteHeader  bool
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // default if WriteHeader is not called
	}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	if lrw.wroteHeader {
		return
	}
	lrw.wroteHeader = true
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !lrw.wroteHeader {
		lrw.WriteHeader(http.StatusOK)
	}
	size, err := lrw.ResponseWriter.Write(b)
	lrw.bytesWritten += size
	return size, err
}

// Unwrap exposes the underlying http.ResponseWriter to http.ResponseController,
// allowing standard library proxies to successfully hijack the connection for
// WebSockets (e.g., Next.js HMR) without breaking byte-tracking middleware.
func (lrw *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return lrw.ResponseWriter
}

// sanitize prevents CRLF log injection by stripping carriage returns and newlines
// from untrusted user input before it is embedded in log messages.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// LoggingMiddleware wraps an http.Handler to provide access logging.
// It sanitizes input to prevent CRLF injection while maintaining a clean,
// structured output aesthetic.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := newLoggingResponseWriter(w)

		next.ServeHTTP(lrw, r)

		duration := time.Since(start).Milliseconds()

		cleanHost := sanitize(r.Host)
		cleanPath := sanitize(r.URL.Path)
		if r.URL.RawQuery != "" {
			cleanPath += "?" + sanitize(r.URL.RawQuery)
		}

		// Subphase 1.7: Smart Framework Noise Filter
		logLvl := "INFO"
		if lrw.statusCode >= 500 {
			logLvl = "ERROR"
		} else if lrw.statusCode >= 400 {
			logLvl = "WARN"
		} else {
			dest := r.Header.Get("Sec-Fetch-Dest")
			conn := strings.ToLower(r.Header.Get("Connection"))
			// Downgrade framework assets and HMR WebSockets to DEBUG.
			if dest == "script" || dest == "style" || dest == "image" || dest == "font" || strings.Contains(conn, "upgrade") {
				logLvl = "DEBUG"
			}
		}

		// Emit: [access] GET api.localhost/v1 → 200 OK (45ms)
		msg := fmt.Sprintf("%s %s%s → %d %s (%dms)",
			r.Method,
			cleanHost,
			cleanPath,
			lrw.statusCode,
			http.StatusText(lrw.statusCode),
			duration,
		)

		accessLog := logger.New("access")
		switch logLvl {
		case "DEBUG":
			accessLog.Debug(msg)
		case "WARN":
			accessLog.Warn(msg)
		case "ERROR":
			accessLog.Error(msg, nil)
		default:
			accessLog.Info(msg)
		}
	})
}
