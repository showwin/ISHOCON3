import bcrypt
import subprocess
from typing import Annotated
import random
from datetime import datetime
from http import HTTPStatus

from fastapi import FastAPI, HTTPException, Response, Depends
from pydantic import BaseModel
from sqlalchemy import text

from .models import Station, Setting, User
from .middlewares import app_auth_middleware
from .sql import engine
from .utils import get_application_clock

app = FastAPI()

class PostInitializeResponse(BaseModel):
    language: str


@app.post("/api/initialize")
def post_initialize() -> PostInitializeResponse:
    result = subprocess.run(
        "/home/ishocon/webapp/sql/init.sh", stdout=subprocess.PIPE, stderr=subprocess.STDOUT
    )
    if result.returncode != 0:
        raise HTTPException(
            status_code=HTTPStatus.INTERNAL_SERVER_ERROR,
            detail=f"Failed to initialize: {result.stdout.decode()}",
        )

    with engine.begin() as conn:
        conn.execute(text("DELETE FROM settings"))
        conn.execute(text("INSERT INTO settings values ()"))

    return PostInitializeResponse(language="python")


class CurrentTimeResponse(BaseModel):
    current_time: str


@app.get("/api/current_time")
def get_current_time():
    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM settings LIMIT 1")
        ).fetchone()
    print(row)
    setting = Setting.model_validate(row)
    return CurrentTimeResponse(
        current_time=get_application_clock(setting.initialized_at)
    )


class StationsResponse(BaseModel):
    stations: list[Station]


@app.get("/api/stations")
def get_stations() -> StationsResponse:
    with engine.begin() as conn:
        rows = conn.execute(
            text("SELECT * FROM stations")
        ).fetchall()
    stations = [Station.model_validate(r) for r in rows]
    return StationsResponse(stations=stations)


@app.get("/api/trains")
def get_trains():
    trains = []
    for i in range(0, 10):
        trains.append({
            "id": i + 1,
            "name": "Train " + str(i + 1),
            "availability": {
                "Arena->Bridge": "lots",
                "Bridge->Cave": "few",
                "Cave->Dock": "none",
                "Dock->Edge": "lots",
                "Edge->Dock": "lots",
                "Dock->Cave": "few",
                "Cave->Bridge": "lots",
                "Bridge->Arena": "none"
            },
            "departure_times": {
                "Arena->Bridge": "12:30",
                "Bridge->Cave": "12:40",
                "Cave->Dock": "12:50",
                "Dock->Edge": "13:00",
                "Edge->Dock": "13:10",
                "Dock->Cave": "13:20",
                "Cave->Bridge": "13:30",
                "Bridge->Arena": "13:40"
            }
        })

    return {"trains": trains}

@app.get("/api/purchased_tickets")
def get_purchased_tickets():
    return {"tickets": [
        {
            "reservation_id": "UUID-1234-5678-91011",
            "train_name": "Train 1",
            "from_station": "Arena",
            "to_station": "Bridge",
            "departure_time": "12:30",
            "seats": ["1-A", "1-B"],
            "total_price": 20000
        },
        {
            "reservation_id": "UUID-9876-5432-91011",
            "train_name": "Train 2",
            "from_station": "Bridge",
            "to_station": "Cave",
            "departure_time": "12:40",
            "seats": ["2-A", "2-B"],
            "total_price": 20000
        }
    ]}

@app.post("/api/reserve")
def post_reserve():
    r = random.randint(0, 10)
    if r < 5:
        return {
            "status": "success",
            "reserved": {
                "reservation_id": "UUID-1234-5678-91011",
                "train_name": "こまち3号",
                "from_station": "Arena",
                "to_station": "Cave",
                "departure_time": "12:30",
                "seats": ["1-A", "1-B"],
                "total_price": 20000,
            }
        }
    elif r < 8:
        return {
            "status": "recommend",
            "recommend": {
                "reservation_id": "UUID-9876-5432-91011",
                "train_name":"こまち3号",
                "from_station": "Arena",
                "to_station": "Cave",
                "departure_time": "12:35",
                "seats":["3-C","3-D"],
                "total_price":18000
            }
        }
    else:
        return {
            "status": "fail",
            "error_code": "NO_SEAT_AVAILABLE"
        }

@app.post("/api/purchase")
def post_purchase():
    return {
        "status": "success",
        "entry_token": "UUID-1234-5678-91011",
        "qr_code_url": "http://localhost/qr20241212060642863.png"
    }

@app.post("/api/entry")
def post_entry():
    return {
        "status": "success",
    }

class LoginRequest(BaseModel):
    name: str
    password: str

## ログインページ
@app.post("/api/login")
def post_login(req: LoginRequest, response: Response):
    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM users WHERE name = :name"),
            {"name": req.name}
        ).fetchone()
    user = User.model_validate(row)

    hashed_password = bcrypt.hashpw(req.password.encode(), user.salt.encode()).decode()
    if user.hashed_password != hashed_password:
        raise HTTPException(
            status_code=HTTPStatus.UNAUTHORIZED,
            detail="Invalid name or password"
        )

    response.set_cookie(key="user_name", value=user.name, httponly=True)

    return {"status": "success", "user": {"id": user.id, "name": user.name, "is_admin": user.is_admin}}

@app.post("/api/logout")
def post_logout(
    _: Annotated[User, Depends(app_auth_middleware)],
    response: Response
):
    response.set_cookie(key="user_name", value=None, httponly=True)
    return {"status": "success"}


## ウェイティングルーム
@app.get("/api/waiting_status")
def get_waiting_status():
    r = random.randint(0, 50)
    if r < 1:
        return {"status": "ready"}
    return {
        "status": "waiting",
        "next_check": 1000
    }

## Admin API
@app.get("/api/admin/stats")
def get_admin_stats():
    return {
        "total_sales": 1000000,
        "total_refunds": 20000
    }

@app.get("/api/admin/trains_sales")
def get_admin_trains_sales():
    return {
        "trains": [
            {
                "train_name": "こまち3号",
                "tickets_sold": 120,
                "pending_revenue": 300000,
                "confirmed_revenue": 200000,
            },
            {
                "train_name": "こまち4号",
                "tickets_sold": 120,
                "pending_revenue": 300000,
                "confirmed_revenue": 200000,
            }
        ]
    }

@app.post("/api/admin/add_train")
def post_add_train():
    return {
        "status": "success",
        "train_name": "こまち5号",
        "departure_time": "12:30",
        "seats": 120
    }
