// Package models holds the JSON types shared by the API layer and the
// on-disk job/render state. Field names and JSON tags mirror the Python
// server's schemas.py so the Expo app keeps working unchanged.
package models

import (
	"errors"
	"net/url"
	"strings"
)

type JobStatus string

const (
	JobPending     JobStatus = "pending"
	JobDownloading JobStatus = "downloading"
	JobProcessing  JobStatus = "processing"
	JobReady       JobStatus = "ready"
	JobFailed      JobStatus = "failed"
)

type RenderStatus string

const (
	RenderPending   RenderStatus = "pending"
	RenderRendering RenderStatus = "rendering"
	RenderReady     RenderStatus = "ready"
	RenderFailed    RenderStatus = "failed"
)

// ErrorCode is a stable, translatable error code. The app maps these to
// Spanish copy; never show raw exception text to the user.
type ErrorCode string

const (
	InvalidURL       ErrorCode = "invalid_url"
	HostNotAllowed   ErrorCode = "host_not_allowed"
	VideoPrivate     ErrorCode = "video_private"
	VideoUnavailable ErrorCode = "video_unavailable"
	VideoTooLarge    ErrorCode = "video_too_large"
	DownloadFailed   ErrorCode = "download_failed"
	ExtractorError   ErrorCode = "extractor_error"
	RenderFailedCode ErrorCode = "render_failed"
	RangeOutOfBounds ErrorCode = "range_out_of_bounds"
	RateLimited      ErrorCode = "rate_limited"
	NotFound         ErrorCode = "not_found"
	Timeout          ErrorCode = "timeout"
	InternalError    ErrorCode = "internal_error"
)

type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Detail  *string   `json:"detail"`
}

type CreateJobRequest struct {
	URL string `json:"url"`
}

// Validate mirrors pydantic's HttpUrl: must parse and be http(s) with a host.
func (r CreateJobRequest) Validate() error {
	u, err := url.Parse(strings.TrimSpace(r.URL))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("url must be a valid http(s) URL")
	}
	return nil
}

type JobMetadata struct {
	Duration float64 `json:"duration"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FPS      float64 `json:"fps"`
}

// JobState is what gets persisted in job.json.
type JobState struct {
	JobID     string       `json:"job_id"`
	Status    JobStatus    `json:"status"`
	Progress  float64      `json:"progress"`
	Metadata  *JobMetadata `json:"metadata"`
	Error     *APIError    `json:"error"`
	UpdatedAt float64      `json:"updated_at"`
}

type JobResponse struct {
	JobID     string       `json:"job_id"`
	Status    JobStatus    `json:"status"`
	Progress  float64      `json:"progress"`
	Metadata  *JobMetadata `json:"metadata"`
	ProxyURL  *string      `json:"proxy_url"`
	SpriteURL *string      `json:"sprite_url"`
	Error     *APIError    `json:"error"`
}

// CropRect is a normalized crop rectangle relative to the source video.
// It may extend outside [0,1] (the editor's zoom/pan UI can ask for a crop
// bigger than the frame); the renderer pads those areas. The bounds below
// are only a generous sanity cap.
type CropRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (c CropRect) Validate() error {
	switch {
	case c.X < -1.5 || c.X > 1.5:
		return errors.New("crop.x out of range")
	case c.Y < -1.5 || c.Y > 1.5:
		return errors.New("crop.y out of range")
	case c.Width <= 0 || c.Width > 3:
		return errors.New("crop.width out of range")
	case c.Height <= 0 || c.Height > 3:
		return errors.New("crop.height out of range")
	}
	return nil
}

type RenderType string

const (
	Animated RenderType = "animated"
	Static   RenderType = "static"
)

type CreateRenderRequest struct {
	Type    RenderType `json:"type"`
	Start   float64    `json:"start"`
	End     *float64   `json:"end"`
	FrameAt *float64   `json:"frame_at"`
	Crop    CropRect   `json:"crop"`
	Emojis  []string   `json:"emojis"`
}

// Validate checks the request and normalizes it: for static renders with no
// frame_at, FrameAt defaults to Start (same as the Python validator).
func (r *CreateRenderRequest) Validate() error {
	if r.Type != Animated && r.Type != Static {
		return errors.New("type must be 'animated' or 'static'")
	}
	if r.Start < 0 {
		return errors.New("start must be >= 0")
	}
	if r.End != nil && *r.End < 0 {
		return errors.New("end must be >= 0")
	}
	if r.FrameAt != nil && *r.FrameAt < 0 {
		return errors.New("frame_at must be >= 0")
	}
	if err := r.Crop.Validate(); err != nil {
		return err
	}
	if len(r.Emojis) > 3 {
		return errors.New("emojis: at most 3")
	}
	if r.Type == Animated {
		if r.End == nil || *r.End <= r.Start {
			return errors.New("animated renders require end > start")
		}
	} else if r.FrameAt == nil {
		v := r.Start
		r.FrameAt = &v
	}
	return nil
}

// RenderFitInfo reports what the budget fitter had to do to hit the size limit.
type RenderFitInfo struct {
	FPS      *float64 `json:"fps"`
	Quality  *int     `json:"quality"`
	Duration *float64 `json:"duration"`
	Degraded bool     `json:"degraded"`
	Note     *string  `json:"note"`
}

// RenderState is what gets persisted in render.json.
type RenderState struct {
	RenderID  string         `json:"render_id"`
	JobID     string         `json:"job_id"`
	Status    RenderStatus   `json:"status"`
	Type      RenderType     `json:"type"`
	Bytes     *int64         `json:"bytes"`
	Fit       *RenderFitInfo `json:"fit"`
	Error     *APIError      `json:"error"`
	UpdatedAt float64        `json:"updated_at"`
}

type RenderResponse struct {
	RenderID   string         `json:"render_id"`
	JobID      string         `json:"job_id"`
	Status     RenderStatus   `json:"status"`
	Type       RenderType     `json:"type"`
	Bytes      *int64         `json:"bytes"`
	StickerURL *string        `json:"sticker_url"`
	TrayURL    *string        `json:"tray_url"`
	Fit        *RenderFitInfo `json:"fit"`
	Error      *APIError      `json:"error"`
}
