import bcrypt
import subprocess
from typing import Annotated
import random
from datetime import datetime, timedelta
from http import HTTPStatus

from fastapi import FastAPI, HTTPException, Response, Depends
from pydantic import BaseModel
from sqlalchemy import text
from ulid import ULID

from .models import Station, User, TrainSchedule, Train, Setting, Reservation, ReservationQrImage, Payment, TrainModel
from .middlewares import app_auth_middleware, admin_auth_middleware
from .sql import engine
from .utils import get_application_clock, get_available_seats_sign, take_lock, release_lock, pick_seats, calculate_seat_price, get_departure_at, release_seat_reservation, generate_qr_image, add_time, update_last_activity_at

app = FastAPI()

WAITING_ROOM_CONFIG = {
    "max_active_users": 5,
    "polling_interval_ms": 500
}

SESSION_CONFIG = {
    "active_time_threshold_sec": 10,
    "polling_interval_ms": 500
}

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


class ScheduleData(BaseModel):
    id: str
    availability: dict[str, str]
    departure_at: dict[str, str]


class ScheduleResponse(BaseModel):
    schedules: list[ScheduleData]


@app.get("/api/schedules")
def get_schedules() -> ScheduleResponse:
    current_time = get_application_clock()
    current_hour, current_minute = current_time.split(":")

    # 入場までのタイムラグを考慮して、3時間後以降のスケジュールを取得している。
    # 本当はもっと直近のものを取得してできるだけ早い時間帯に乗車してもらいたい
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
            "departure_at": {
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

class TicketData(BaseModel):
    reservation_id: str
    schedule_id: str
    from_station: str
    to_station: str
    departure_at: str
    seats: list[str]
    total_price: int
    entry_token: str
    qr_code_url: str
    is_entered: bool


class PurchasedTicketsResponse(BaseModel):
    tickets: list[TicketData]


@app.get("/api/purchased_tickets")
def get_purchased_tickets(
    user: Annotated[User, Depends(app_auth_middleware)],
) -> PurchasedTicketsResponse:
    tickets = []
    with engine.begin() as conn:
        rows = conn.execute(
            text("""
                 SELECT
                    r.id as reservation_id,
                    r.schedule_id ,
                    s1.name as from_station,
                    s2.name as to_station,
                    r.departure_at,
                    p.amount as total_price,
                    r.entry_token,
                    qr.id as qr_id,
                    CASE
                        WHEN e.id IS NOT NULL THEN 1
                        ELSE 0
                    END AS is_entered
                 FROM reservations r
                 INNER JOIN payments p ON p.reservation_id = r.id
                 INNER JOIN reservation_qr_images qr ON qr.reservation_id = r.id
                 INNER JOIN stations s1 ON r.from_station_id = s1.id
                 INNER JOIN stations s2 ON r.to_station_id = s2.id
                 LEFT OUTER JOIN entries e ON e.reservation_id = r.id
                 WHERE r.user_id = :user_id
                 AND p.is_captured = true
                 """),
            {"user_id": user.id}
        ).fetchall()
        for r in rows:
            seat_rows = conn.execute(
                text("SELECT seat FROM reservation_seats WHERE reservation_id = :reservation_id"),
                {"reservation_id": r[0]}
            ).fetchall()
            ticket = TicketData(
                reservation_id=r[0],
                schedule_id=r[1],
                from_station=r[2],
                to_station=r[3],
                departure_at=r[4],
                seats=[s[0] for s in seat_rows],
                total_price=r[5],
                entry_token=r[6],
                qr_code_url=f"http://localhost:8080/api/qr/{r[7]}.png",
                is_entered=r[8]
            )
            tickets.append(ticket)

    # このAPIは画面リロード時に呼ばれるので、ユーザはアクティブだと判断してユーザーの最終アクティビティを更新する
    update_last_activity_at(user.id)

    return PurchasedTicketsResponse(tickets=tickets)


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
    departure_at: str
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
    update_last_activity_at(user.id)

    if not take_lock(req.schedule_id):
        return {"status": "fail", "error_code": "LOCK_TIMEOUT"}

    reserved_schedule_id, seats = pick_seats(req.schedule_id, req.from_station_id, req.to_station_id, req.num_people)

    if reserved_schedule_id is None:
        return {"status": "fail", "error_code": "NO_SEAT_AVAILABLE"}

    departure_at = get_departure_at(reserved_schedule_id, req.from_station_id, req.to_station_id)
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
            departure_at=reservation.departure_at,
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

    update_last_activity_at(user.id)

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
    update_last_activity_at(user.id)

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
        row = conn.execute(
            text("SELECT * FROM entries WHERE reservation_id = :reservation_id"),
            {"reservation_id": reservation.id}
        ).fetchone()
    if row is not None:
        raise HTTPException(
            status_code=HTTPStatus.BAD_REQUEST,
            detail="Already entered"
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


class SessionResponse(BaseModel):
    status: str
    next_check: int

@app.get("/api/session")
def get_session(
    user: Annotated[User, Depends(app_auth_middleware)],
    res: Response
) -> SessionResponse:
    # 開発中に短時間でセッションが切れるのは不便なので、ishoconユーザはセッションを切れないようにしておくのがおすすめ
    # if user.name == "ishocon":
    #     return SessionResponse(
    #         status="active",
    #         next_check=9999999999
    #     )

    # `active_time_threshold_sec` 秒以上アクティブでないユーザはログアウトさせる
    if user.last_activity_at < (datetime.now() - timedelta(seconds=SESSION_CONFIG["active_time_threshold_sec"])):
        res.set_cookie(key="user_name", value=None, httponly=True)
        return SessionResponse(
            status="session_expired",
            next_check=SESSION_CONFIG["polling_interval_ms"]
        )
    return SessionResponse(
        status="active",
        next_check=SESSION_CONFIG["polling_interval_ms"]
    )


## ログインページ

class LoginRequest(BaseModel):
    name: str
    password: str


class UserData(BaseModel):
    id: str
    name: str
    is_admin: bool


class LoginResponse(BaseModel):
    status: str
    user: UserData


@app.post("/api/login")
def post_login(req: LoginRequest, response: Response) -> LoginResponse:
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

    if user.is_admin:
        response.set_cookie(key="admin_name", value=user.name, httponly=True)
    else:
        response.set_cookie(key="user_name", value=user.name, httponly=True)

    update_last_activity_at(user.id)

    return LoginResponse(
        status="success",
        user=UserData(id=user.id, name=user.name, is_admin=user.is_admin)
    )


@app.post("/api/logout")
def post_logout(
    _: Annotated[User, Depends(app_auth_middleware)],
    response: Response
):
    response.set_cookie(key="user_name", value=None, httponly=True)
    return {"status": "success"}


## Waiting Room

class WaitingStatusResponse(BaseModel):
    status: str
    next_check: int


@app.get("/api/waiting_status")
def get_waiting_status(
    user: Annotated[User, Depends(app_auth_middleware)],
) -> WaitingStatusResponse:

    update_last_activity_at(user.id)

    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT count(*) as active_user_count FROM users WHERE last_activity_at = :threshold"),
            {"threshold": datetime.now() - timedelta(seconds=SESSION_CONFIG["active_time_threshold_sec"])}
        ).fetchone()
    active_user_count = row[0]

    if active_user_count >= WAITING_ROOM_CONFIG["max_active_users"]:
        status = "waiting"
    else:
        status = "ready"

    return WaitingStatusResponse(
        status=status,
        next_check=WAITING_ROOM_CONFIG["polling_interval_ms"]
    )

## Admin API

class StatsResponse(BaseModel):
    total_sales: int
    total_refunds: int


@app.get("/api/admin/stats")
def get_admin_stats(
    _: Annotated[User, Depends(admin_auth_middleware)],
) -> StatsResponse:
    with engine.begin() as conn:
        row = conn.execute(
            text("""
                 SELECT SUM(amount) AS total_sales
                 FROM payments
                 INNER JOIN entries
                 ON payments.reservation_id = entries.reservation_id
                 WHERE is_captured = 1
                 """),
        ).fetchone()
        total_sales = row[0] if row[0] else 0

        row = conn.execute(
            text("""
                SELECT SUM(amount) AS total_refunds
                FROM payments
                WHERE is_refunded = 1
                """),
        ).fetchone()
        total_refunds = row[0] if row[0] else 0

    return StatsResponse(
        total_sales=total_sales,
        total_refunds=total_refunds
    )


class TrainSalesData(BaseModel):
    train_name: str
    tickets_sold: int
    pending_revenue: int
    confirmed_revenue: int
    refunds: int


class TrainSalesResponse(BaseModel):
    trains: list[TrainSalesData]


@app.get("/api/admin/train_sales")
def get_admin_train_sales():
    with engine.begin() as conn:
        rows = conn.execute(
            text("""
                 SELECT
                    t.name as train_name,
                    SUM(CASE WHEN p.is_captured THEN 1 ELSE 0 END) as tickets_sold,
                    SUM(CASE WHEN e.id IS NULL AND p.is_captured THEN p.amount ELSE 0 END) as pending_revenue,
                    SUM(CASE WHEN e.id IS NOT NULL AND p.is_captured THEN p.amount ELSE 0 END) as confirmed_revenue,
                    SUM(CASE WHEN p.is_refunded THEN p.amount ELSE 0 END) as refunds
                 FROM trains t
                 INNER JOIN train_schedules s ON t.id = s.train_id
                 INNER JOIN reservations r ON s.id = r.schedule_id
                 INNER JOIN payments p ON r.id = p.reservation_id
                 LEFT OUTER JOIN entries e ON r.id = e.reservation_id
                 GROUP BY t.name
                 """),
        ).fetchall()
        train_sales = [TrainSalesData(
            train_name=r[0],
            tickets_sold=r[1],
            pending_revenue=r[2],
            confirmed_revenue=r[3],
            refunds=r[4]
        ) for r in rows]
    return TrainSalesResponse(trains=train_sales)


class TrainData(BaseModel):
    model_names: list[str]


@app.get("/api/train_models")
def get_train_models() -> TrainData:
    with engine.begin() as conn:
        rows = conn.execute(
            text("SELECT * FROM train_models")
        ).fetchall()
    train_models = [TrainModel.model_validate(r) for r in rows]
    return TrainData(model_names=[m.name for m in train_models])


class AddTrainRequest(BaseModel):
    train_name: str
    model_name: str
    departure_times: list[str]


class AddTrainResponse(BaseModel):
    status: str


@app.post("/api/admin/add_train")
def post_add_train(req: AddTrainRequest) -> AddTrainResponse:
    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM train_models WHERE name = :name"),
            {"name": req.model_name}
        ).fetchone()
    if row is None:
        return HTTPException(status_code=HTTPStatus.BAD_REQUEST)

    with engine.begin() as conn:
        conn.execute(
            text("""
                INSERT INTO trains (name, model_name)
                VALUES (:name, :model_name)
                """),
            {"name": req.train_name, "model_name": req.model_name}
        )

        row = conn.execute(
            text("SELECT * FROM trains WHERE name = :name"),
            {"name": req.train_name}
        ).fetchone()
    train = Train.model_validate(row)

    with engine.begin() as conn:
        for i, departure_time_at_a in enumerate(req.departure_times):
            conn.execute(
                text("""
                    INSERT INTO train_schedules
                    (id,
                     train_id,
                     departure_at_station_a_to_b,
                     departure_at_station_b_to_c,
                     departure_at_station_c_to_d,
                     departure_at_station_d_to_e,
                     departure_at_station_e_to_d,
                     departure_at_station_d_to_c,
                     departure_at_station_c_to_b,
                     departure_at_station_b_to_a)
                    VALUES
                    (:id,
                     :train_id,
                     :departure_at_station_a_to_b,
                     :departure_at_station_b_to_c,
                     :departure_at_station_c_to_d,
                     :departure_at_station_d_to_e,
                     :departure_at_station_e_to_d,
                     :departure_at_station_d_to_c,
                     :departure_at_station_c_to_b,
                     :departure_at_station_b_to_a)
                    """),
                {
                    "id": f"{train.name}-{i + 1}",
                    "train_id": train.id,
                    "departure_at_station_a_to_b": departure_time_at_a,
                    "departure_at_station_b_to_c": add_time(departure_time_at_a, 10),
                    "departure_at_station_c_to_d": add_time(departure_time_at_a, 20),
                    "departure_at_station_d_to_e": add_time(departure_time_at_a, 30),
                    "departure_at_station_e_to_d": add_time(departure_time_at_a, 40),
                    "departure_at_station_d_to_c": add_time(departure_time_at_a, 50),
                    "departure_at_station_c_to_b": add_time(departure_time_at_a, 60),
                    "departure_at_station_b_to_a": add_time(departure_time_at_a, 70),
                }
            )

    with engine.begin() as conn:
        rows = conn.execute(
            text("SELECT * FROM train_schedules WHERE train_id = :train_id"),
            {"train_id": train.id}
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
    return AddTrainResponse(status="success")
