"""FastAPI application entrypoint."""
from __future__ import annotations

import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from app.api import jobs, renders
from app.core.config import get_settings
from app.core.errors import AppError
from app.core.queue import WorkQueues
from app.core.ratelimit import RateLimiter
from app.core.storage import JobStore, run_cleanup_loop

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
logger = logging.getLogger("stickers")


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings = get_settings()
    settings.storage_dir.mkdir(parents=True, exist_ok=True)

    app.state.settings = settings
    app.state.store = JobStore(settings)
    app.state.queues = WorkQueues(settings)
    app.state.rate_limiter = RateLimiter(settings.rate_limit_per_hour)

    cleanup_task = asyncio.create_task(run_cleanup_loop(app.state.store, settings))
    logger.info("stickers-server starting up, storage_dir=%s", settings.storage_dir)
    try:
        yield
    finally:
        cleanup_task.cancel()
        try:
            await cleanup_task
        except asyncio.CancelledError:
            pass


app = FastAPI(title="Animated Stickers Maker API", version="0.1.0", lifespan=lifespan)

# Wide open CORS: this API has no auth/cookies, is called from a mobile app
# (not a browser origin that needs protecting), and is meant to also be
# poked at with curl during development.
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.exception_handler(AppError)
async def handle_app_error(request: Request, exc: AppError) -> JSONResponse:
    return JSONResponse(
        status_code=exc.status_code,
        content={"code": exc.code, "message": exc.message, "detail": exc.detail},
    )


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}


app.include_router(jobs.router)
app.include_router(renders.router)
