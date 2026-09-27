"""Render endpoints: turn a job's video + a chosen crop/time range into a
WhatsApp-compliant sticker (animated or static), plus its tray icon.
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
    CreateRenderRequest,
    ErrorCode,
    JobStatus,
    RenderFitInfo,
    RenderResponse,
    RenderStatus,
)
from app.services.webp import crop_to_pixels, fit_animated, fit_static, render_tray_icon

logger = logging.getLogger("stickers.api.renders")

router = APIRouter(tags=["renders"])


def _to_response(state: dict) -> RenderResponse:
    render_id = state["render_id"]
    status = RenderStatus(state["status"])
    sticker_url = f"/api/v1/renders/{render_id}/sticker.webp" if status == RenderStatus.READY else None
    tray_url = f"/api/v1/renders/{render_id}/tray.webp" if status == RenderStatus.READY else None
    fit = RenderFitInfo(**state["fit"]) if state.get("fit") else None
    error = ApiError(**state["error"]) if state.get("error") else None
    return RenderResponse(
        render_id=render_id,
        job_id=state["job_id"],
        status=status,
        type=state["type"],
        bytes=state.get("bytes"),
        sticker_url=sticker_url,
        tray_url=tray_url,
        fit=fit,
        error=error,
    )


async def _process_render(
    render_id: str, job_id: str, body: CreateRenderRequest, store: JobStore, settings: Settings
) -> None:
    render_dir = settings.render_dir(render_id)
    try:
        await store.update_render(render_id, status=RenderStatus.RENDERING)

        job_state = await store.read_job(job_id)
        if job_state is None or job_state.get("status") != JobStatus.READY:
            raise AppError(ErrorCode.NOT_FOUND, "El job no existe o todavía no está listo.", status_code=404)

        source = store.find_source_file(job_id)
        if source is None:
            raise AppError(ErrorCode.NOT_FOUND, "No se encontró el video fuente.", status_code=404)

        metadata = job_state["metadata"]
        crop_px = crop_to_pixels(body.crop, metadata["width"], metadata["height"])

        end = body.end if body.type == "animated" else None
        frame_at = body.frame_at if body.frame_at is not None else body.start
        if body.start >= metadata["duration"] or (end is not None and end > metadata["duration"] + 0.25):
            raise AppError(ErrorCode.RANGE_OUT_OF_BOUNDS, "El recorte elegido está fuera del video.")

        sticker_path = render_dir / "sticker.webp"
        tray_path = render_dir / "tray.webp"

        if body.type == "animated":
            fit = await fit_animated(
                source, sticker_path, body.start, end, crop_px,
                metadata["width"], metadata["height"], settings,
            )
        else:
            fit = await fit_static(
                source, sticker_path, frame_at, crop_px,
                metadata["width"], metadata["height"], settings,
            )

        await render_tray_icon(
            source, tray_path, frame_at, crop_px, metadata["width"], metadata["height"], settings,
        )

        await store.update_render(
            render_id,
            status=RenderStatus.READY,
            bytes=sticker_path.stat().st_size,
            fit=fit.model_dump(),
        )
    except AppError as exc:
        logger.warning("render %s failed: %s (%s)", render_id, exc.message, exc.detail)
        await store.update_render(
            render_id,
            status=RenderStatus.FAILED,
            error=ApiError(code=exc.code, message=exc.message, detail=exc.detail).model_dump(),
        )
    except Exception as exc:  # noqa: BLE001
        logger.exception("render %s crashed", render_id)
        await store.update_render(
            render_id,
            status=RenderStatus.FAILED,
            error=ApiError(
                code=ErrorCode.INTERNAL_ERROR, message="Ocurrió un error inesperado.", detail=str(exc)
            ).model_dump(),
        )


@router.post(
    "/api/v1/jobs/{job_id}/renders",
    response_model=RenderResponse,
    dependencies=[Depends(enforce_rate_limit)],
)
async def create_render(
    job_id: str,
    body: CreateRenderRequest,
    store: JobStore = Depends(get_store),
    settings: Settings = Depends(get_settings),
    queues: WorkQueues = Depends(get_queues),
) -> RenderResponse:
    job_state = await store.read_job(job_id)
    if job_state is None:
        raise AppError(ErrorCode.NOT_FOUND, "No se encontró el job.", status_code=404)
    if job_state.get("status") != JobStatus.READY:
        raise AppError(ErrorCode.NOT_FOUND, "El job todavía no está listo para generar un sticker.")

    render_id = new_id()
    store.create_render_dir(render_id)
    initial = {
        "render_id": render_id,
        "job_id": job_id,
        "status": RenderStatus.PENDING,
        "type": body.type,
    }
    await store.write_render(render_id, initial)

    async def _task() -> None:
        await queues.run_render(lambda: _process_render(render_id, job_id, body, store, settings))

    asyncio.create_task(_task())

    return _to_response(initial)


@router.get("/api/v1/renders/{render_id}", response_model=RenderResponse)
async def get_render(render_id: str, store: JobStore = Depends(get_store)) -> RenderResponse:
    state = await store.read_render(render_id)
    if state is None:
        raise AppError(ErrorCode.NOT_FOUND, "No se encontró el render.", status_code=404)
    return _to_response(state)


@router.get("/api/v1/renders/{render_id}/sticker.webp")
async def get_sticker(render_id: str, settings: Settings = Depends(get_settings)) -> FileResponse:
    path = settings.render_dir(render_id) / "sticker.webp"
    if not path.exists():
        raise AppError(ErrorCode.NOT_FOUND, "El sticker todavía no está listo.", status_code=404)
    return FileResponse(path, media_type="image/webp")


@router.get("/api/v1/renders/{render_id}/tray.webp")
async def get_tray(render_id: str, settings: Settings = Depends(get_settings)) -> FileResponse:
    path = settings.render_dir(render_id) / "tray.webp"
    if not path.exists():
        raise AppError(ErrorCode.NOT_FOUND, "El ícono todavía no está listo.", status_code=404)
    return FileResponse(path, media_type="image/webp")
