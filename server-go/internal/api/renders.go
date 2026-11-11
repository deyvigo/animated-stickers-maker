package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"stickers-server/internal/apperr"
	"stickers-server/internal/models"
	"stickers-server/internal/sticker"
	"stickers-server/internal/storage"
)

func renderResponse(st *models.RenderState) models.RenderResponse {
	resp := models.RenderResponse{
		RenderID: st.RenderID,
		JobID:    st.JobID,
		Status:   st.Status,
		Type:     st.Type,
		Bytes:    st.Bytes,
		Fit:      st.Fit,
		Error:    st.Error,
	}
	if st.Status == models.RenderReady {
		stickerURL := "/api/v1/renders/" + st.RenderID + "/sticker.webp"
		trayURL := "/api/v1/renders/" + st.RenderID + "/tray.webp"
		resp.StickerURL, resp.TrayURL = &stickerURL, &trayURL
	}
	return resp
}

func (s *Server) createRender(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r) {
		return
	}
	jobID := r.PathValue("job_id")
	var req models.CreateRenderRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		invalidRequest(w, err.Error())
		return
	}

	if !storage.ValidID(jobID) {
		writeError(w, notFound("No se encontró el job."))
		return
	}
	job, err := s.Store.ReadJob(jobID)
	if err != nil {
		writeError(w, err)
		return
	}
	if job == nil {
		writeError(w, notFound("No se encontró el job."))
		return
	}
	if job.Status != models.JobReady {
		writeError(w, apperr.New(models.NotFound, "El job todavía no está listo para generar un sticker."))
		return
	}

	renderID := storage.NewID()
	if _, err := s.Store.CreateRenderDir(renderID); err != nil {
		writeError(w, err)
		return
	}
	pending := models.RenderState{
		RenderID: renderID, JobID: jobID, Status: models.RenderPending, Type: req.Type,
	}
	if err := s.Store.WriteRender(pending); err != nil {
		writeError(w, err)
		return
	}

	s.spawn(func(ctx context.Context) {
		err := s.Queues.RunRender(ctx, func(ctx context.Context) { s.processRender(ctx, renderID, jobID, req) })
		if err != nil {
			s.failRender(renderID, err)
		}
	})

	writeJSON(w, http.StatusOK, renderResponse(&pending))
}

func (s *Server) getRender(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("render_id")
	if !storage.ValidID(id) {
		writeError(w, notFound("No se encontró el render."))
		return
	}
	st, err := s.Store.ReadRender(id)
	if err != nil {
		writeError(w, err)
		return
	}
	if st == nil {
		writeError(w, notFound("No se encontró el render."))
		return
	}
	writeJSON(w, http.StatusOK, renderResponse(st))
}

func (s *Server) getSticker(w http.ResponseWriter, r *http.Request) {
	s.serveRenderFile(w, r, "sticker.webp", "El sticker todavía no está listo.")
}

func (s *Server) getTray(w http.ResponseWriter, r *http.Request) {
	s.serveRenderFile(w, r, "tray.webp", "El ícono todavía no está listo.")
}

func (s *Server) serveRenderFile(w http.ResponseWriter, r *http.Request, name, notReady string) {
	id := r.PathValue("render_id")
	if !storage.ValidID(id) {
		writeError(w, notFound(notReady))
		return
	}
	serveFile(w, r, filepath.Join(s.Settings.RenderDir(id), name), "image/webp", notReady)
}

func (s *Server) setRender(renderID string, fn func(*models.RenderState)) {
	if _, err := s.Store.UpdateRender(renderID, fn); err != nil {
		slog.Error("update render failed", "render", renderID, "err", err)
	}
}

func (s *Server) failRender(renderID string, err error) {
	ae := apperr.From(err)
	switch {
	case errors.Is(err, context.Canceled):
		ae = apperr.New(models.InternalError, "El servidor se detuvo antes de terminar el render.")
		slog.Warn("render interrupted by shutdown", "render", renderID)
	case ae.Code == models.InternalError:
		slog.Error("render crashed", "render", renderID, "err", err)
	default:
		slog.Warn("render failed", "render", renderID, "message", ae.Message, "detail", ae.Detail)
	}
	s.setRender(renderID, func(r *models.RenderState) {
		r.Status = models.RenderFailed
		r.Error = ae.ToAPI()
	})
}

func (s *Server) processRender(ctx context.Context, renderID, jobID string, req models.CreateRenderRequest) {
	if err := s.runRender(ctx, renderID, jobID, req); err != nil {
		s.failRender(renderID, err)
	}
}

func (s *Server) runRender(ctx context.Context, renderID, jobID string, req models.CreateRenderRequest) error {
	s.setRender(renderID, func(r *models.RenderState) { r.Status = models.RenderRendering })

	job, err := s.Store.ReadJob(jobID)
	if err != nil {
		return err
	}
	if job == nil || job.Status != models.JobReady || job.Metadata == nil {
		return notFound("El job no existe o todavía no está listo.")
	}
	source := s.Store.FindSourceFile(jobID)
	if source == "" {
		return notFound("No se encontró el video fuente.")
	}

	md := job.Metadata
	crop := sticker.CropToPixels(req.Crop, md.Width, md.Height)

	frameAt := req.Start
	if req.FrameAt != nil {
		frameAt = *req.FrameAt
	}
	animated := req.Type == models.Animated
	// Small tolerance on the end: the editor's slider can land a hair past
	// the probed duration.
	outOfBounds := req.Start >= md.Duration
	for _, sg := range req.Segments { // Validate fills these for animated renders
		if sg.Start >= md.Duration || sg.End > md.Duration+0.25 {
			outOfBounds = true
		}
	}
	if outOfBounds {
		return apperr.New(models.RangeOutOfBounds, "El recorte elegido está fuera del video.")
	}

	dir := s.Settings.RenderDir(renderID)
	stickerPath := filepath.Join(dir, "sticker.webp")
	trayPath := filepath.Join(dir, "tray.webp")

	var fit models.RenderFitInfo
	if animated {
		fit, err = s.Renderer.FitAnimated(ctx, source, stickerPath, req.Segments, crop, md.Width, md.Height, sticker.PaddingFill(req.Background), req.Speed)
	} else {
		fit, err = s.Renderer.FitStatic(ctx, source, stickerPath, frameAt, crop, md.Width, md.Height, sticker.PaddingFill(req.Background))
	}
	if err != nil {
		return err
	}
	if err := s.Renderer.RenderTray(ctx, source, trayPath, frameAt, crop, md.Width, md.Height, sticker.PaddingFill(req.Background)); err != nil {
		return err
	}

	st, err := os.Stat(stickerPath)
	if err != nil {
		return err
	}
	size := st.Size()
	s.setRender(renderID, func(r *models.RenderState) {
		r.Status, r.Bytes, r.Fit = models.RenderReady, &size, &fit
	})
	return nil
}
