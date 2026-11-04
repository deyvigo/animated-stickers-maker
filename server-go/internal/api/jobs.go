package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"stickers-server/internal/apperr"
	"stickers-server/internal/download"
	"stickers-server/internal/media"
	"stickers-server/internal/models"
	"stickers-server/internal/storage"
)

func jobResponse(st *models.JobState) models.JobResponse {
	resp := models.JobResponse{
		JobID:    st.JobID,
		Status:   st.Status,
		Progress: st.Progress,
		Metadata: st.Metadata,
		Error:    st.Error,
	}
	if st.Status == models.JobReady {
		proxy := "/api/v1/jobs/" + st.JobID + "/proxy.mp4"
		sprite := "/api/v1/jobs/" + st.JobID + "/sprite.jpg"
		resp.ProxyURL, resp.SpriteURL = &proxy, &sprite
	}
	return resp
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r) {
		return
	}
	var req models.CreateJobRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		invalidRequest(w, err.Error())
		return
	}
	url := strings.TrimSpace(req.URL)
	if err := download.AssertAllowedHost(url, s.Settings.AllowedHosts); err != nil {
		writeError(w, err)
		return
	}

	jobID := storage.NewID()
	if _, err := s.Store.CreateJobDir(jobID); err != nil {
		writeError(w, err)
		return
	}
	pending := models.JobState{JobID: jobID, Status: models.JobPending}
	if err := s.Store.WriteJob(pending); err != nil {
		writeError(w, err)
		return
	}

	s.spawn(func(ctx context.Context) {
		err := s.Queues.RunDownload(ctx, func(ctx context.Context) { s.processJob(ctx, jobID, url) })
		if err != nil { // shutdown while still queued
			s.failJob(jobID, err)
		}
	})

	writeJSON(w, http.StatusOK, jobResponse(&pending))
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("job_id")
	if !storage.ValidID(id) {
		writeError(w, notFound("No se encontró el job."))
		return
	}
	st, err := s.Store.ReadJob(id)
	if err != nil {
		writeError(w, err)
		return
	}
	if st == nil {
		writeError(w, notFound("No se encontró el job."))
		return
	}
	writeJSON(w, http.StatusOK, jobResponse(st))
}

func (s *Server) getProxy(w http.ResponseWriter, r *http.Request) {
	s.serveJobFile(w, r, "proxy.mp4", "video/mp4")
}

func (s *Server) getSprite(w http.ResponseWriter, r *http.Request) {
	s.serveJobFile(w, r, "sprite.jpg", "image/jpeg")
}

func (s *Server) serveJobFile(w http.ResponseWriter, r *http.Request, name, contentType string) {
	id := r.PathValue("job_id")
	const notReady = "La vista previa todavía no está lista."
	if !storage.ValidID(id) {
		writeError(w, notFound(notReady))
		return
	}
	serveFile(w, r, filepath.Join(s.Settings.JobDir(id), name), contentType, notReady)
}

// setJob applies fn to the job state, logging (not returning) store errors:
// there is no caller to report them to once the job runs in the background.
func (s *Server) setJob(jobID string, fn func(*models.JobState)) {
	if _, err := s.Store.UpdateJob(jobID, fn); err != nil {
		slog.Error("update job failed", "job", jobID, "err", err)
	}
}

func (s *Server) failJob(jobID string, err error) {
	ae := apperr.From(err)
	switch {
	case errors.Is(err, context.Canceled):
		ae = apperr.New(models.InternalError, "El servidor se detuvo antes de terminar el job.")
		slog.Warn("job interrupted by shutdown", "job", jobID)
	case ae.Code == models.InternalError:
		slog.Error("job crashed", "job", jobID, "err", err)
	default:
		slog.Warn("job failed", "job", jobID, "message", ae.Message, "detail", ae.Detail)
	}
	s.setJob(jobID, func(j *models.JobState) {
		j.Status = models.JobFailed
		j.Error = ae.ToAPI()
	})
}

// processJob downloads the video and builds the editor's preview assets.
func (s *Server) processJob(ctx context.Context, jobID, url string) {
	if err := s.runJob(ctx, jobID, url); err != nil {
		s.failJob(jobID, err)
	}
}

func (s *Server) runJob(ctx context.Context, jobID, url string) error {
	dir := s.Settings.JobDir(jobID)

	s.setJob(jobID, func(j *models.JobState) { j.Status, j.Progress = models.JobDownloading, 0.1 })
	if err := s.Downloader.Download(ctx, jobID, url); err != nil {
		return err
	}
	source := s.Store.FindSourceFile(jobID)
	if source == "" {
		return apperr.New(models.DownloadFailed, "El video se procesó pero no se encontró el archivo.")
	}

	s.setJob(jobID, func(j *models.JobState) { j.Status, j.Progress = models.JobProcessing, 0.5 })
	md, err := media.ProbeVideo(ctx, source)
	if err != nil {
		return err
	}

	timeout := s.Settings.FFmpegTimeout
	if err := media.GenerateProxy(ctx, source, filepath.Join(dir, "proxy.mp4"), timeout); err != nil {
		return err
	}
	s.setJob(jobID, func(j *models.JobState) { j.Progress = 0.75 })
	if err := media.GenerateSprite(ctx, source, filepath.Join(dir, "sprite.jpg"), md.Duration, timeout); err != nil {
		return err
	}

	s.setJob(jobID, func(j *models.JobState) {
		j.Status, j.Progress, j.Metadata = models.JobReady, 1.0, &md
	})
	return nil
}
