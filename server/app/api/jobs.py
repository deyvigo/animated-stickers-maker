"""Job endpoints: create a job from a TikTok URL, poll its status, and
fetch the preview assets (proxy video + thumbnail sprite) it produces.
"""
from __future__ import annotations

import asyncio
import logging

from fastapi import APIRouter, Depends
from fastapi.responses import FileResponse

from app.core.config import Settings
from app.core.deps import enforce_rate_limit, get_queues, get_settings, get_store
from app.core.errors import AppError
from app.core.queue import WorkQueues
from app.core.storage import JobStore, new_id
from app.models.schemas import (
    ApiError,
    CreateJobRequest,
    ErrorCode,
    JobMetadata,
    JobResponse,
    JobStatus,
)
from app.services import download, preview, probe

logger = logging.getLogger("stickers.api.jobs")

router = APIRouter(prefix="/api/v1/jobs", tags=["jobs"])


def _to_response(state: dict, settings: Settings) -> JobResponse:
    job_id = state["job_id"]
    status = JobStatus(state["status"])
    proxy_url = f"/api/v1/jobs/{job_id}/proxy.mp4" if status == JobStatus.READY else None
    sprite_url = f"/api/v1/jobs/{job_id}/sprite.jpg" if status == JobStatus.READY else None
    metadata = JobMetadata(**state["metadata"]) if state.get("metadata") else None
    error = ApiError(**state["error"]) if state.get("error") else None
    return JobResponse(
        job_id=job_id,
        status=status,
        progress=state.get("progress", 0.0),
        metadata=metadata,
        proxy_url=proxy_url,
        sprite_url=sprite_url,
        error=error,
    )


async def _process_job(job_id: str, url: str, store: JobStore, settings: Settings) -> None:
    job_dir = settings.job_dir(job_id)
    try:
        await store.update_job(job_id, status=JobStatus.DOWNLOADING, progress=0.1)
        source = await download.download_video(url, job_dir, settings)

        await store.update_job(job_id, status=JobStatus.PROCESSING, progress=0.5)
        metadata = await probe.probe_video(source)

        proxy_path = job_dir / "proxy.mp4"
        sprite_path = job_dir / "sprite.jpg"
        await preview.generate_proxy(source, proxy_path, settings.ffmpeg_timeout_seconds)
        await store.update_job(job_id, progress=0.75)
        await preview.generate_sprite(
            source, sprite_path, metadata.duration, settings.ffmpeg_timeout_seconds
        )

        await store.update_job(
            job_id,
            status=JobStatus.READY,
            progress=1.0,
            metadata=metadata.model_dump(),
        )
    except AppError as exc:
        logger.warning("job %s failed: %s (%s)", job_id, exc.message, exc.detail)
        await store.update_job(
            job_id,
            status=JobStatus.FAILED,
            error=ApiError(code=exc.code, message=exc.message, detail=exc.detail).model_dump(),
        )
    except Exception as exc:  # noqa: BLE001
        logger.exception("job %s crashed", job_id)
        await store.update_job(
            job_id,
            status=JobStatus.FAILED,
            error=ApiError(
                code=ErrorCode.INTERNAL_ERROR, message="Ocurrió un error inesperado.", detail=str(exc)
            ).model_dump(),
        )


@router.post("", response_model=JobResponse, dependencies=[Depends(enforce_rate_limit)])
async def create_job(
    body: CreateJobRequest,
    store: JobStore = Depends(get_store),
    settings: Settings = Depends(get_settings),
    queues: WorkQueues = Depends(get_queues),
) -> JobResponse:
    url = str(body.url)
    download.assert_allowed_host(url, settings)

    job_id = new_id()
    store.create_job_dir(job_id)
    await store.write_job(job_id, {"job_id": job_id, "status": JobStatus.PENDING, "progress": 0.0})

    async def _task() -> None:
        await queues.run_download(lambda: _process_job(job_id, url, store, settings))

    asyncio.create_task(_task())

    return _to_response({"job_id": job_id, "status": JobStatus.PENDING, "progress": 0.0}, settings)


@router.get("/{job_id}", response_model=JobResponse)
async def get_job(
    job_id: str,
    store: JobStore = Depends(get_store),
    settings: Settings = Depends(get_settings),
) -> JobResponse:
    state = await store.read_job(job_id)
    if state is None:
        raise AppError(ErrorCode.NOT_FOUND, "No se encontró el job.", status_code=404)
    return _to_response(state, settings)


@router.get("/{job_id}/proxy.mp4")
async def get_proxy(job_id: str, settings: Settings = Depends(get_settings)) -> FileResponse:
    path = settings.job_dir(job_id) / "proxy.mp4"
    if not path.exists():
        raise AppError(ErrorCode.NOT_FOUND, "La vista previa todavía no está lista.", status_code=404)
    return FileResponse(path, media_type="video/mp4")


@router.get("/{job_id}/sprite.jpg")
async def get_sprite(job_id: str, settings: Settings = Depends(get_settings)) -> FileResponse:
    path = settings.job_dir(job_id) / "sprite.jpg"
    if not path.exists():
        raise AppError(ErrorCode.NOT_FOUND, "La vista previa todavía no está lista.", status_code=404)
    return FileResponse(path, media_type="image/jpeg")
