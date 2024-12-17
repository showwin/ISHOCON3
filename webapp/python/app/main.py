import bcrypt
import subprocess
from typing import Annotated
import random
from datetime import datetime
from http import HTTPStatus

from fastapi import FastAPI, HTTPException, Response, Depends
from pydantic import BaseModel
from sqlalchemy import text
from ulid import ULID

from .models import Station, User, TrainSchedule, Train, Setting, Reservation, ReservationQrImage, Payment
from .middlewares import app_auth_middleware
from .sql import engine
from .utils import get_application_clock, get_available_seats_sign, take_lock, release_lock, pick_seats, calculate_seat_price, get_departure_time, release_seat_reservation, generate_qr_image

app = FastAPI()

class PostInitializeResponse(BaseModel):
    initialized_at: datetime
    app_language: str
    ui_language: str


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
        row = conn.execute(text("SELECT * FROM settings")).fetchone()
        setting = Setting.model_validate(row)

    return PostInitializeResponse(
        initialized_at=setting.initialized_at,
        app_language="python",
        ui_language="ja"
    )


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


@app.get("/api/schedules")
def get_schedules():
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
                        AND from_station_id = :from_station
                        AND to_station_id = :to_station
                    """),
                    {"schedule_id": schedule.id, "from_station": stations.split("->")[0], "to_station": stations.split("->")[1]},
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
            available_seats_between_stations[stations] = get_available_seats_sign(available_seats, total_seats)

        trains.append({
            "id": schedule.id,
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

    return {"schedules": trains}

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

class PostReserveRequest(BaseModel):
    schedule_id: str
    from_station_id: str
    to_station_id: str
    num_people: int

class ReservationData(BaseModel):
    reservation_id: str
    schedule_id: str
    from_station: str
    to_station: str
    departure_time: str
    seats: list[str]
    total_price: int
    is_discounted: bool

class PostReserveResponse(BaseModel):
    status: str
    reserved: ReservationData | None = None
    recommend: ReservationData | None = None
    error_code: str | None = None

@app.post("/api/reserve")
def post_reserve(
    user: Annotated[User, Depends(app_auth_middleware)],
    req: PostReserveRequest
) -> PostReserveResponse:
    if not take_lock(req.schedule_id):
        return {"status": "fail", "error_code": "LOCK_TIMEOUT"}

    reserved_schedule_id, seats = pick_seats(req.schedule_id, req.from_station_id, req.to_station_id, req.num_people)

    if reserved_schedule_id is None:
        return {"status": "fail", "error_code": "NO_SEAT_AVAILABLE"}

    departure_at = get_departure_time(reserved_schedule_id, req.from_station_id, req.to_station_id)
    with engine.begin() as conn:
        reservation_id = ULID()
        conn.execute(
            text("""
                 INSERT INTO reservations (id, user_id, schedule_id, from_station_id, to_station_id, departure_at, entry_token)
                 VALUES (:id, :user_id, :schedule_id, :from_station_id, :to_station_id, :departure_at, :entry_token)
                 """),
            {
                "id": reservation_id,
                "user_id": user.id,
                "schedule_id": reserved_schedule_id,
                "from_station_id": req.from_station_id,
                "to_station_id": req.to_station_id,
                "departure_at": departure_at,
                "entry_token": str(ULID())
            }
        )
    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM reservations WHERE id = :id"),
            {"id": reservation_id}
        ).fetchone()
        reservation = Reservation.model_validate(row)

        for seat in seats:
            conn.execute(
                text("""
                    INSERT INTO reservation_seats (reservation_id, seat)
                    VALUES (:reservation_id, :seat)
                    """),
                {"reservation_id": reservation.id, "seat": seat}
            )

    with engine.begin() as conn:
        total_price, is_discounted = calculate_seat_price(reservation, seats)
        conn.execute(
            text("""
                INSERT INTO payments (user_id, reservation_id, amount)
                VALUES (:user_id, :reservation_id, :amount)
                """),
            {"user_id": user.id, "reservation_id": reservation.id, "amount": total_price}
        )

    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM stations WHERE id = :id"),
            {"id": reservation.from_station_id}
        ).fetchone()
        from_station = Station.model_validate(row)
        row = conn.execute(
            text("SELECT * FROM stations WHERE id = :id"),
            {"id": reservation.to_station_id}
        ).fetchone()
        to_station = Station.model_validate(row)

    return PostReserveResponse(
        status="success" if reservation.schedule_id == req.schedule_id else "recommend",
        reserved=ReservationData(
            reservation_id=reservation.id,
            schedule_id=reservation.schedule_id,
            from_station=from_station.name,
            to_station=to_station.name,
            departure_time=reservation.departure_at,
            seats=seats,
            total_price=total_price,
            is_discounted=is_discounted
        )
    )

class PostPurchaseRequest(BaseModel):
    reservation_id: str

class PostPurchaseResponse(BaseModel):
    status: str
    entry_token: str
    qr_code_url: str

@app.post("/api/purchase")
def post_purchase(
    user: Annotated[User, Depends(app_auth_middleware)],
    req: PostPurchaseRequest
) -> PostPurchaseResponse:
    # FIXME: レコメンドした場合はチケットが購入されない可能性があるので予約のロックが残ってしまうが、それが発生するケースは少ないと信じて一旦放置

    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM reservations WHERE id = :id"),
            {"id": req.reservation_id}
        ).fetchone()
    reservation = Reservation.model_validate(row)

    if reservation.user_id != user.id:
        raise HTTPException(
            status_code=HTTPStatus.UNAUTHORIZED,
            detail="Invalid reservation"
        )

    # TODO: Payment API をコールする
    i = random.randint(0, 10)
    payment_status = "success" if i > 1 else "failed"

    release_lock(reservation.schedule_id)
    if payment_status == "success":
        with engine.begin() as conn:
            conn.execute(
                text("UPDATE payments SET is_captured = true WHERE reservation_id = :reservation_id"),
                {"reservation_id": reservation.id}
            )
    else:
        release_seat_reservation(reservation)

    qr_image = generate_qr_image(reservation.entry_token)

    image_id = str(ULID())
    with engine.begin() as conn:
        conn.execute(
            text("""
                INSERT INTO reservation_qr_images
                VALUES (:id, :reservation_id, :qr_image)
                """),
            {"id": image_id, "reservation_id": reservation.id, "qr_image": qr_image}
        )

    return PostPurchaseResponse(
        status=payment_status,
        entry_token=reservation.entry_token if payment_status == "success" else "",
        qr_code_url=f"http://localhost:8080/api/qr/{image_id}.png" if payment_status == "success" else ""
    )

@app.get("/api/qr/{qr_id}.png")
def get_qr(qr_id: str):
    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM reservation_qr_images WHERE id = :id"),
            {"id": qr_id}
        ).fetchone()
    if row is None:
        raise HTTPException(status_code=HTTPStatus.NOT_FOUND)
    qr_image = ReservationQrImage.model_validate(row)
    return Response(content=qr_image.image, media_type="image/png")


class PostEntryRequest(BaseModel):
    entry_token: str

class PostEntryResponse(BaseModel):
    status: str

@app.post("/api/entry")
def post_entry(
    user: Annotated[User, Depends(app_auth_middleware)],
    req: PostEntryRequest
) -> PostEntryResponse:
    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM reservations WHERE entry_token = :entry_token"),
            {"entry_token": req.entry_token}
        ).fetchone()
    if row is None:
        return HTTPException(status_code=HTTPStatus.NOT_FOUND)

    reservation = Reservation.model_validate(row)

    if reservation.user_id != user.id:
        return HTTPException(status_code=HTTPStatus.UNAUTHORIZED)
    # 列車の発車時間を過ぎていないことを確認
    if reservation.departure_at < get_application_clock():
        return PostEntryResponse(
            status="TRAIN_DEPARTED",
        )

    with engine.begin() as conn:
        conn.execute(
            text("""
                INSERT INTO entries (reservation_id)
                VALUES (:reservation_id)
                """),
            {"reservation_id": reservation.id}
        )
    return PostEntryResponse(status="success")


class PostRefundRequest(BaseModel):
    reservation_id: str


class PostRefundResponse(BaseModel):
    status: str


@app.post("/api/refund")
def post_refund(
    user: Annotated[User, Depends(app_auth_middleware)],
    req: PostRefundRequest
) -> PostRefundResponse:
    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM reservations WHERE id = :id"),
            {"id": req.reservation_id}
        ).fetchone()
    reservation = Reservation.model_validate(row)

    if reservation.user_id != user.id:
        raise HTTPException(
            status_code=HTTPStatus.UNAUTHORIZED,
            detail="Invalid reservation"
        )

    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM payments WHERE reservation_id = :reservation_id"),
            {"reservation_id": reservation.id}
        ).fetchone()
    payment = Payment.model_validate(row)

    if not payment.is_captured:
        raise HTTPException(
            status_code=HTTPStatus.BAD_REQUEST,
            detail="Not captured"
        )

    with engine.begin() as conn:
        conn.execute(
            text("UPDATE payments SET is_captured = false, is_refunded = true WHERE reservation_id = :reservation_id"),
            {"reservation_id": reservation.id}
        )

    if reservation.departure_at > get_application_clock():
        release_seat_reservation(reservation)

    return PostRefundResponse(
        status="success"
    )


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
    r = random.randint(0, 10)
    if r < 1:
        return {"status": "ready"}
    return {
        "status": "waiting",
        "next_check": 500
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
                            (train_id, schedule_id, from_station_id, to_station_id, seat_row, a_is_available, b_is_available, c_is_available, d_is_available, e_is_available)
                            VALUES (:train_id, :schedule_id, :from_station_id, :to_station_id, :seat_row, :a, :b, :c, :d, :e)
                            """),
                        {
                            "train_id": schedule.train_id,
                            "schedule_id": schedule.id,
                            "from_station_id": stations[0],
                            "to_station_id": stations[1],
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
