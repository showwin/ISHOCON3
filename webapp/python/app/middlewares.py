from http import HTTPStatus
from typing import Annotated

from fastapi import Cookie, HTTPException
from sqlalchemy import text

from .models import User
from .sql import engine


def app_auth_middleware(user_name: Annotated[str | None, Cookie()] = None) -> User:
    if not user_name:
        raise HTTPException(status_code=HTTPStatus.UNAUTHORIZED, detail="user_name cookie is required")

    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM users WHERE name = :name"),
            {"name": user_name},
        ).fetchone()

        if row is None:
            raise HTTPException(status_code=HTTPStatus.UNAUTHORIZED, detail="Invalid user name")
        user = User.model_validate(row)

        return user


def admin_auth_middleware(
    admin_name: Annotated[str | None, Cookie()] = None,
) -> User:
    if not admin_name:
        raise HTTPException(
            status_code=HTTPStatus.UNAUTHORIZED,
            detail="admin_name cookie is required",
        )

    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM users WHERE name = :name and is_admin = 1"),
            {"name": admin_name},
        ).fetchone()

        if row is None:
            raise HTTPException(status_code=HTTPStatus.UNAUTHORIZED, detail="Invalid admin name")

        return User.model_validate(row)
