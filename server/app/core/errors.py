"""Typed application errors.

Every failure that can reach the API boundary should be one of these, not a
bare exception — the app needs a stable `code` to show translated, useful
copy instead of "algo salió mal".
"""
from __future__ import annotations

from app.models.schemas import ErrorCode


class AppError(Exception):
    def __init__(self, code: ErrorCode, message: str, detail: str | None = None, *, status_code: int = 400):
        self.code = code
        self.message = message
        self.detail = detail
        self.status_code = status_code
        super().__init__(message)
