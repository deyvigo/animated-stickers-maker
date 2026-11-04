package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"stickers-server/internal/apperr"
	"stickers-server/internal/models"
)

const maxBodyBytes = 1 << 20

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("write response failed", "err", err)
	}
}

// writeError renders any error in the {code, message, detail} shape the
// app parses. Non-AppErrors become internal_error.
func writeError(w http.ResponseWriter, err error) {
	ae := apperr.From(err)
	if ae.Code == models.InternalError {
		slog.Error("unhandled error", "err", err)
	}
	writeJSON(w, ae.StatusCode, ae.ToAPI())
}

// invalidRequest mirrors the 422 the Python server returned for body
// validation failures, in the same {code, message, detail} shape.
func invalidRequest(w http.ResponseWriter, msg string) {
	d := msg
	writeJSON(w, http.StatusUnprocessableEntity, models.APIError{
		Code: "invalid_request", Message: msg, Detail: &d,
	})
}

func notFound(msg string) *apperr.Error {
	return apperr.New(models.NotFound, msg).WithStatus(http.StatusNotFound)
}

// decodeBody reads a JSON body into v; on failure it writes the 422 and
// returns false.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		invalidRequest(w, "Solicitud inválida: "+err.Error())
		return false
	}
	return true
}

// serveFile streams a stored artifact with Range support (the video
// player needs it for seeking).
func serveFile(w http.ResponseWriter, r *http.Request, path, contentType, notReady string) {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		writeError(w, notFound(notReady))
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeFile(w, r, path)
}

// ---- middleware ---------------------------------------------------------

// Wide-open CORS: the API has no auth/cookies, is called from a mobile app
// and is meant to be poked at with curl during development.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "*")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
				slog.Error("panic in handler", "path", r.URL.Path, "panic", rec)
				writeError(w, errors.New("panic"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController (and ServeContent) reach the real writer.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"ms", time.Since(start).Milliseconds())
	})
}
