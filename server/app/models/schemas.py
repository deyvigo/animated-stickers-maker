"""Pydantic models shared across the API layer."""
from __future__ import annotations

from enum import StrEnum
from typing import Literal

from pydantic import BaseModel, Field, HttpUrl, model_validator


class JobStatus(StrEnum):
    PENDING = "pending"
    DOWNLOADING = "downloading"
    PROCESSING = "processing"
    READY = "ready"
    FAILED = "failed"


class RenderStatus(StrEnum):
    PENDING = "pending"
    RENDERING = "rendering"
    READY = "ready"
    FAILED = "failed"


class ErrorCode(StrEnum):
    """Stable, translatable error codes. The app maps these to Spanish
    copy; never show raw exception text to the user."""

    INVALID_URL = "invalid_url"
    HOST_NOT_ALLOWED = "host_not_allowed"
    VIDEO_PRIVATE = "video_private"
    VIDEO_UNAVAILABLE = "video_unavailable"
    VIDEO_TOO_LARGE = "video_too_large"
    DOWNLOAD_FAILED = "download_failed"
    EXTRACTOR_ERROR = "extractor_error"
    RENDER_FAILED = "render_failed"
    RANGE_OUT_OF_BOUNDS = "range_out_of_bounds"
    RATE_LIMITED = "rate_limited"
    NOT_FOUND = "not_found"
    TIMEOUT = "timeout"
    INTERNAL_ERROR = "internal_error"


class ApiError(BaseModel):
    code: ErrorCode
    message: str
    detail: str | None = None


class CreateJobRequest(BaseModel):
    url: HttpUrl


class JobMetadata(BaseModel):
    duration: float
    width: int
    height: int
    fps: float


class JobResponse(BaseModel):
    job_id: str
    status: JobStatus
    progress: float = 0.0
    metadata: JobMetadata | None = None
    proxy_url: str | None = None
    sprite_url: str | None = None
    error: ApiError | None = None


class CropRect(BaseModel):
    """Normalized crop rectangle, all values in [0, 1] relative to the
    original video's width/height. The backend converts to pixels."""

    x: float = Field(ge=0, le=1)
    y: float = Field(ge=0, le=1)
    width: float = Field(gt=0, le=1)
    height: float = Field(gt=0, le=1)


class CreateRenderRequest(BaseModel):
    type: Literal["animated", "static"]
    start: float = Field(ge=0)
    end: float | None = Field(default=None, ge=0)
    frame_at: float | None = Field(default=None, ge=0)
    crop: CropRect
    emojis: list[str] = Field(default_factory=list, max_length=3)

    @model_validator(mode="after")
    def _check_range(self) -> "CreateRenderRequest":
        if self.type == "animated":
            if self.end is None or self.end <= self.start:
                raise ValueError("animated renders require end > start")
        else:
            if self.frame_at is None:
                self.frame_at = self.start
        return self


class RenderFitInfo(BaseModel):
    """What the budget fitter actually had to do to hit the size limit."""

    fps: float | None = None
    quality: int | None = None
    duration: float | None = None
    degraded: bool = False
    note: str | None = None


class RenderResponse(BaseModel):
    render_id: str
    job_id: str
    status: RenderStatus
    type: Literal["animated", "static"]
    bytes: int | None = None
    sticker_url: str | None = None
    tray_url: str | None = None
    fit: RenderFitInfo | None = None
    error: ApiError | None = None
