import bcrypt
import subprocess
from typing import Annotated
import random
from datetime import datetime
from http import HTTPStatus

from fastapi import FastAPI, HTTPException, Response, Depends
from pydantic import BaseModel
from sqlalchemy import text

from .models import Station, User, TrainSchedule, Train, Setting
from .middlewares import app_auth_middleware
from .sql import engine
from .utils import get_application_clock, get_available_seats_sign

app = FastAPI()

class PostInitializeResponse(BaseModel):
    initialized_at: datetime
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
        row = conn.execute(text("SELECT * FROM settings LIMIT 1")).fetchone()
    setting = Setting.model_validate(row)

    return PostInitializeResponse(initialized_at=setting.initialized_at, language="python")


class CurrentTimeResponse(BaseModel):
    current_time: str


@app.get("/api/current_time")
def get_current_time():
    return CurrentTimeResponse(
        current_time=get_application_clock()
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
    current_time = get_application_clock()
    current_hour, current_minute = current_time.split(":")
    three_hours_later = f"{(int(current_hour) + 3):02d}:{current_minute}"

    with engine.begin() as conn:
        rows = conn.execute(
            text("""
            SELECT *
            FROM train_schedules
            WHERE departure_at_station_a_to_b > :time
            ORDER BY departure_at_station_a_to_b
            LIMIT 10
            """),
            {"time": three_hours_later}
        ).fetchall()
    schedules = [TrainSchedule.model_validate(r) for r in rows]


    trains = []
    for schedule in schedules:
        with engine.begin() as conn:
            row = conn.execute(
                text("SELECT * FROM trains WHERE id = :id"),
                {"id": schedule.train_id},
            ).fetchone()
        train = Train.model_validate(row)

        available_seats_between_stations = {
            "A->B": 0,
            "B->C": 0,
            "C->D": 0,
            "D->E": 0,
            "E->D": 0,
            "D->C": 0,
            "C->B": 0,
            "B->A": 0,
        }
        for stations in available_seats_between_stations.keys():
            with engine.begin() as conn:
                row = conn.execute(
                    text("""
                        SELECT SUM(a_is_available) + SUM(b_is_available) + SUM(c_is_available) + SUM(d_is_available) + SUM(e_is_available) AS available_seats
                        FROM seat_row_reservations
                        WHERE schedule_id = :schedule_id
                        AND station_from_id = :station_from
                        AND station_to_id = :station_to
                    """),
                    {"schedule_id": schedule.id, "station_from": stations.split("->")[0], "station_to": stations.split("->")[1]},
                ).fetchone()
                available_seats = row[0]

                row = conn.execute(
                    text("""
                        SELECT seat_rows * seat_columns AS total_seats
                        FROM trains
                        INNER JOIN train_models
                        ON trains.model_name = train_models.name
                        WHERE id = :train_id;
                    """),
                    {"train_id": train.id},
                ).fetchone()
                total_seats = row[0]
            available_seats_between_stations[stations] = get_available_seats_sign(total_seats, available_seats)

        trains.append({
            "id": train.id,
            "name": train.name,
            "availability": {
                "Arena->Bridge": available_seats_between_stations["A->B"],
                "Bridge->Cave": available_seats_between_stations["B->C"],
                "Cave->Dock": available_seats_between_stations["C->D"],
                "Dock->Edge": available_seats_between_stations["D->E"],
                "Edge->Dock": available_seats_between_stations["E->D"],
                "Dock->Cave": available_seats_between_stations["D->C"],
                "Cave->Bridge": available_seats_between_stations["C->B"],
                "Bridge->Arena": available_seats_between_stations["B->A"],
            },
            "departure_times": {
                "Arena->Bridge": schedule.departure_at_station_a_to_b,
                "Bridge->Cave": schedule.departure_at_station_b_to_c,
                "Cave->Dock": schedule.departure_at_station_c_to_d,
                "Dock->Edge": schedule.departure_at_station_d_to_e,
                "Edge->Dock": schedule.departure_at_station_e_to_d,
                "Dock->Cave": schedule.departure_at_station_d_to_c,
                "Cave->Bridge": schedule.departure_at_station_c_to_b,
                "Bridge->Arena": schedule.departure_at_station_b_to_a
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


## ログインページ

class LoginRequest(BaseModel):
    name: str
    password: str

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
    with engine.begin() as conn:
        rows = conn.execute(
            text("select * from train_schedules"),
        ).fetchall()

    schedules = [TrainSchedule.model_validate(r) for r in rows]

    with engine.begin() as conn:
        for schedule in schedules:
            row = conn.execute(
                text("SELECT * FROM trains WHERE id = :id"),
                {"id": schedule.train_id},
            ).fetchone()
            train = Train.model_validate(row)
            row = conn.execute(
                text("SELECT * FROM train_models WHERE name = :model_name"),
                {"model_name": train.model_name},
            ).fetchone()
            train_model = TrainModel.model_validate(row)

            for i in range(train_model.seat_rows):
                stations_list = [["A", "B"], ["B", "C"], ["C", "D"], ["D", "E"], ["E", "D"], ["D", "C"], ["C", "B"], ["B", "A"]]
                for stations in stations_list:
                    conn.execute(
                        text("""
                            INSERT INTO seat_row_reservations
                            (train_id, schedule_id, station_from_id, station_to_id, seat_row, a_is_available, b_is_available, c_is_available, d_is_available, e_is_available)
                            VALUES (:train_id, :schedule_id, :station_from_id, :station_to_id, :seat_row, :a, :b, :c, :d, :e)
                            """),
                        {
                            "train_id": schedule.train_id,
                            "schedule_id": schedule.id,
                            "station_from_id": stations[0],
                            "station_to_id": stations[1],
                            "seat_row": i + 1,
                            "a": 1 if train_model.seat_columns >= 1 else 0,
                            "b": 1 if train_model.seat_columns >= 2 else 0,
                            "c": 1 if train_model.seat_columns >= 3 else 0,
                            "d": 1 if train_model.seat_columns >= 4 else 0,
                            "e": 1 if train_model.seat_columns >= 5 else 0,
                        },
                    )
    return {
        "status": "success",
        "train_name": "こまち5号",
        "departure_time": "12:30",
        "seats": 120
    }
