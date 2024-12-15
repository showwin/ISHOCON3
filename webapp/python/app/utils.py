from datetime import datetime
from enum import Enum
import time
import math

from sqlalchemy import text
from sqlalchemy.exc import IntegrityError

from .models import BaseModel, Setting, Reservation, SeatRowReservation, TrainSchedule, ReservationSeat
from .sql import engine

BASE_TICKET_PRICE = 1000

class AvailableSeats(Enum):
    LOTS = "lots"
    FEW = "few"
    NONE = "none"

def get_application_clock() -> str:
    with engine.begin() as conn:
        row = conn.execute(
            text("SELECT * FROM settings LIMIT 1")
        ).fetchone()
    setting = Setting.model_validate(row)

    time_passed = datetime.now() - setting.initialized_at
    # この世界では1秒が10分に相当する
    # 24:00で世界が止まる
    hours = min(time_passed.seconds // 6, 24)
    minutes = time_passed.seconds % 6 * 10 if hours < 24 else 0
    return f"{hours:02d}:{minutes:02d}"

def get_available_seats_sign(available_seats: int, total_seats: int) -> str:
    if available_seats == 0:
        return AvailableSeats.NONE.value
    if available_seats / total_seats <= 0.1:
        return AvailableSeats.FEW.value
    return AvailableSeats.LOTS.value


def take_lock(schedule_id: str) -> bool:
    print(f"take_lock: {schedule_id}")
    retry = 10
    i = 0
    with engine.begin() as conn:
        while True:
            try:
                conn.execute(
                    text("INSERT INTO reservation_locks (schedule_id) VALUES (:id)"),
                    {"id": schedule_id}
                )
            except IntegrityError:
                time.sleep(0.1)
                i += 1
                if i > retry:
                    return False
            else:
                break
    return True

def release_lock(schedule_id: str) -> None:
    with engine.begin() as conn:
        conn.execute(
            text("DELETE FROM reservation_locks WHERE schedule_id = :id"),
            {"id": schedule_id}
        )

class SeatRowStatus(BaseModel):
    seat_row: int
    a: int
    b: int
    c: int
    d: int
    e: int

def pick_seats(schedule_id: str, from_station_id: str, to_station_id: str, num_people: int) -> tuple[str | None, list[str]]:
    # 乗車区間を考えるのは大変なので、最初から最後まで全部空いているかどうかだけを考える
    # 本当は乗車区間だけステータスを更新したい…

    # 全区間空いている席が num_people 以上あるかどうかを確認する
    with engine.begin() as conn:
        available_seats = conn.execute(
            text("""
                SELECT SUM(a + b + c + d + e) as total_available_seats
                FROM (SELECT seat_row, MIN(a_is_available) AS a, MIN(b_is_available) AS b, MIN(c_is_available) AS c, MIN(d_is_available) AS d, MIN(e_is_available) AS e
                      FROM seat_row_reservations
                      WHERE schedule_id = :schedule_id GROUP BY seat_row) as available_seats;
                 """),
            {"schedule_id": schedule_id}
        ).fetchone()[0]
    if available_seats < num_people:
        release_lock(schedule_id)
        # レコメンド用の空席探し
        with engine.begin() as conn:
            row = conn.execute(
                text("""
                    SELECT * FROM train_schedules WHERE id = :id
                    """),
                {"id": schedule_id}
            ).fetchone()
            schedule = TrainSchedule.model_validate(row)
            rows = conn.execute(
                text("""
                    SELECT *
                    FROM train_schedules
                    WHERE departure_at_station_a_to_b > :departure_at_station_a_to_b
                    ORDER BY departure_at_station_a_to_b
                    LIMIT 1
                    """),
                {"departure_at_station_a_to_b": schedule.departure_at_station_a_to_b}
            ).fetchone()
            if rows is None:
                return None, []
            next_schedule = TrainSchedule.model_validate(row)
            take_lock(next_schedule.id)
            return pick_seats(next_schedule.id, from_station_id, to_station_id, num_people)

    with engine.begin() as conn:
        rows = conn.execute(
            text("""
                SELECT seat_row, MIN(a_is_available) AS a, MIN(b_is_available) AS b, MIN(c_is_available) AS c, MIN(d_is_available) AS d, MIN(e_is_available) AS e
                FROM seat_row_reservations
                WHERE schedule_id = :schedule_id GROUP BY seat_row
                """),
            {"schedule_id": schedule_id}
        ).fetchall()
    seat_rows = [SeatRowStatus.model_validate(row) for row in rows]
    reserved_seats = []
    for _ in range(num_people):
        for seat_row in seat_rows:
            if seat_row.a:
                reserved_seats.append(f"{seat_row.seat_row}-A")
                seat_row.a = False
                break
            if seat_row.b:
                reserved_seats.append(f"{seat_row.seat_row}-B")
                seat_row.b = False
                break
            if seat_row.c:
                reserved_seats.append(f"{seat_row.seat_row}-C")
                seat_row.c = False
                break
            if seat_row.d:
                reserved_seats.append(f"{seat_row.seat_row}-D")
                seat_row.d = False
                break
            if seat_row.e:
                reserved_seats.append(f"{seat_row.seat_row}-E")
                seat_row.e = False
                break

    # 予約状況を反映
    for seat in reserved_seats:
        seat_row, column = seat.split("-")
        with engine.begin() as conn:
            conn.execute(
                text(f"""
                    UPDATE seat_row_reservations
                    SET {column.lower()}_is_available = 0
                    WHERE schedule_id = :schedule_id AND seat_row = :seat_row
                    """),
                {"schedule_id": schedule_id, "seat_row": seat_row}
            )

    return schedule_id, reserved_seats


def get_stations_between(start: str, end: str) -> list[str]:
    stations = ["A", "B", "C", "D", "E", "Dr", "Cr", "Br", "Ar"]
    station_ids = ["A", "B", "C", "D", "E", "D", "C", "B", "A"]
    if start > end:
        end = end + "r"

    start_index = stations.index(start)
    end_index = stations[start_index:].index(end) + start_index
    return station_ids[start_index:end_index + 1]

def calculate_seat_price(reservation: Reservation, seats: list[str]) -> tuple[int, bool]:
    distance = calculate_distance(reservation.from_station_id, reservation.to_station_id)
    num_seats = len(seats)
    if num_seats == 1:
        return BASE_TICKET_PRICE * distance, False

    with engine.begin() as conn:
        row = conn.execute(
            text("""
                 SELECT seat_columns
                 FROM train_models tm
                 INNER JOIN trains t ON t.model_name = tm.name
                 INNER JOIN train_schedules ts ON ts.train_id = t.id
                 INNER JOIN reservations r ON r.schedule_id = ts.id
                 WHERE r.id = :reservation_id
                 """),
            {"reservation_id": reservation.id}
        ).fetchone()
        train_seat_columns = row[0]

    allowed_groups = math.ceil(num_seats / train_seat_columns)
    seats = sorted(seats)
    full_price = BASE_TICKET_PRICE * distance * num_seats
    discounted_price = full_price * 0.5

    # 必要以上に席が違う列に分かれてしまっている場合は割引料金
    seat_rows = len(set([s.split("-")[0] for s in seats]))
    if seat_rows > allowed_groups:
        print(f"more than allowed groups. {seat_rows} > {allowed_groups} = {num_seats} / {train_seat_columns}. ")
        return discounted_price, True

    seat_column_list = ["A", "B", "C", "D", "E"]
    previous_seat = None
    for seat in seats:
        if previous_seat is None:
            previous_seat = seat
            continue
        previous_row, previous_column = previous_seat.split("-")
        row, column = seat.split("-")
        if row == previous_row:
            expected_column = seat_column_list[seat_column_list.index(previous_column) + 1]
            if column == expected_column:
                previous_seat = seat
                continue
            else:
                print("not next to each other")
                # 同じ列だが席が隣り合っていない場合は割引料金
                return discounted_price, True
        previous_seat = seat

    return full_price, False

def calculate_distance(start, end):
    stations = ["A", "B", "C", "D", "E", "Dr", "Cr", "Br", "Ar"]
    if start > end:
        end = end + "r"

    start_index = stations.index(start)
    end_index = stations[start_index:].index(end) + start_index
    return end_index - start_index

def get_departure_time(schedule_id: str, from_station_id: str, to_station_id: str) -> str:
    stations = get_stations_between(from_station_id, to_station_id)
    next_station = stations[1]

    with engine.begin() as conn:
        row = conn.execute(
            text(f"""
                SELECT * FROM train_schedules WHERE id = :id
                """),
            {"id": schedule_id}
        ).fetchone()
    schedule = TrainSchedule.model_validate(row)
    return getattr(schedule, f"departure_at_station_{from_station_id.lower()}_to_{next_station.lower()}")
