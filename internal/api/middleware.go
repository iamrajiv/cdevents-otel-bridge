/*
Package api provides the HTTP API for the CDEvents-OTel Bridge.

This file implements the middleware chain: structured request logging (with
the chi request id and matched route pattern), panic recovery that turns a
crash into a JSON 500, and permissive CORS so browser dashboards can query
the read endpoints.
*/
package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			route := r.URL.Path
			if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
				route = rctx.RoutePattern()
			}

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"route", route,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

func RecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if err, ok := rec.(error); ok && err == http.ErrAbortHandler {
						panic(rec)
					}
					logger.Error("panic recovered",
						"error", rec,
						"stack", string(debug.Stack()),
						"path", r.URL.Path,
						"method", r.Method,
						"request_id", middleware.GetReqID(r.Context()),
					)
					respondError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
