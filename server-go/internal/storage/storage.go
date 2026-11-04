// Package storage is the filesystem-backed job/render state. Each job is a
// directory containing job.json plus its media; each render likewise. This
// is the only place that reads/writes those files, so state can't drift
// from what's on disk.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"stickers-server/internal/config"
	"stickers-server/internal/models"
)

// NewID returns a random 32-char hex id (same shape as uuid4().hex).
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	return hex.EncodeToString(b[:])
}

// ValidID reports whether id looks like something NewID produced. Handlers
// must check this before building paths from URL parameters, to rule out
// path traversal ("../").
func ValidID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

type Store struct {
	settings config.Settings
	// One mutex serializes all state-file writes, ruling out
	// read-modify-write races on job.json / render.json. With a handful
	// of concurrent jobs, contention is a non-issue.
	mu sync.Mutex
}

func New(settings config.Settings) *Store {
	return &Store{settings: settings}
}

func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// ---- jobs ---------------------------------------------------------------

func (s *Store) CreateJobDir(jobID string) (string, error) {
	dir := s.settings.JobDir(jobID)
	if err := os.MkdirAll(filepath.Join(dir, "renders"), 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func (s *Store) WriteJob(state models.JobState) error {
	dir := s.settings.JobDir(state.JobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	state.UpdatedAt = now()
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicWriteJSON(filepath.Join(dir, "job.json"), state)
}

// ReadJob returns (nil, nil) when the job doesn't exist.
func (s *Store) ReadJob(jobID string) (*models.JobState, error) {
	var st models.JobState
	ok, err := readJSON(filepath.Join(s.settings.JobDir(jobID), "job.json"), &st)
	if err != nil || !ok {
		return nil, err
	}
	return &st, nil
}

// UpdateJob applies fn to the current state under the lock and persists it.
// A missing job starts from {JobID: jobID}.
func (s *Store) UpdateJob(jobID string, fn func(*models.JobState)) (*models.JobState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.settings.JobDir(jobID), "job.json")
	st := models.JobState{JobID: jobID}
	if _, err := readJSON(path, &st); err != nil {
		return nil, err
	}
	fn(&st)
	st.UpdatedAt = now()
	if err := atomicWriteJSON(path, st); err != nil {
		return nil, err
	}
	return &st, nil
}

// FindSourceFile returns the downloaded source video path, or "" if none.
func (s *Store) FindSourceFile(jobID string) string {
	matches, _ := filepath.Glob(filepath.Join(s.settings.JobDir(jobID), "source.*"))
	for _, m := range matches {
		if filepath.Ext(m) != ".tmp" {
			return m
		}
	}
	return ""
}

// ---- renders ------------------------------------------------------------

func (s *Store) CreateRenderDir(renderID string) (string, error) {
	dir := s.settings.RenderDir(renderID)
	return dir, os.MkdirAll(dir, 0o755)
}

func (s *Store) WriteRender(state models.RenderState) error {
	dir := s.settings.RenderDir(state.RenderID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	state.UpdatedAt = now()
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomicWriteJSON(filepath.Join(dir, "render.json"), state)
}

// ReadRender returns (nil, nil) when the render doesn't exist.
func (s *Store) ReadRender(renderID string) (*models.RenderState, error) {
	var st models.RenderState
	ok, err := readJSON(filepath.Join(s.settings.RenderDir(renderID), "render.json"), &st)
	if err != nil || !ok {
		return nil, err
	}
	return &st, nil
}

func (s *Store) UpdateRender(renderID string, fn func(*models.RenderState)) (*models.RenderState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.settings.RenderDir(renderID), "render.json")
	st := models.RenderState{RenderID: renderID}
	if _, err := readJSON(path, &st); err != nil {
		return nil, err
	}
	fn(&st)
	st.UpdatedAt = now()
	if err := atomicWriteJSON(path, st); err != nil {
		return nil, err
	}
	return &st, nil
}

// ---- TTL cleanup --------------------------------------------------------

// SweepExpired deletes job and render directories whose mtime is older than
// the configured TTL. Returns how many were removed.
func (s *Store) SweepExpired() int {
	cutoff := time.Now().Add(-s.settings.JobTTL)
	removed := 0
	for _, root := range []string{"jobs", "renders"} {
		entries, err := os.ReadDir(filepath.Join(s.settings.StorageDir, root))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			full := filepath.Join(s.settings.StorageDir, root, e.Name())
			info, err := os.Stat(full)
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				if err := os.RemoveAll(full); err == nil {
					removed++
				}
			}
		}
	}
	return removed
}

// RunCleanupLoop sweeps expired entries periodically until done is closed.
func (s *Store) RunCleanupLoop(done <-chan struct{}) {
	t := time.NewTicker(s.settings.CleanupInterval)
	defer t.Stop()
	for {
		if n := s.SweepExpired(); n > 0 {
			slog.Info("cleanup: removed expired entries", "count", n)
		}
		select {
		case <-done:
			return
		case <-t.C:
		}
	}
}

// ---- helpers ------------------------------------------------------------

// readJSON decodes path into v. Returns (false, nil) if the file doesn't exist.
func readJSON(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	return true, nil
}

// atomicWriteJSON writes to a temp file and renames over the target, so
// readers never see a half-written file.
func atomicWriteJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
