from datetime import datetime

import pydantic


class BaseModel(pydantic.BaseModel):
    model_config = pydantic.ConfigDict(from_attributes=True)


class Station(BaseModel):
    id: str
    name: str


class Setting(BaseModel):
    initialized_at: datetime


class User(BaseModel):
    id: str
    name: str
    hashed_password: str
    salt: str
    is_admin: bool
    global_payment_token: str
    last_activity_at: datetime | None
    created_at: datetime


class TrainModel(BaseModel):
    name: str
    seat_rows: int
    seat_columns: int


class Train(BaseModel):
    id: int
    name: str
    model: str


class TrainSchedule(BaseModel):
    id: str
    train_id: int
    departure_at_station_a_to_b: str
    departure_at_station_b_to_c: str
    departure_at_station_c_to_d: str
    departure_at_station_d_to_e: str
    departure_at_station_e_to_d: str
    departure_at_station_d_to_c: str
    departure_at_station_c_to_b: str
    departure_at_station_b_to_a: str


class SeatRowReservation(BaseModel):
    id: int
    train_id: int
    schedule_id: str
    from_station_id: str
    to_station_id: str
    seat_row: int
    a_is_available: bool
    b_is_available: bool
    c_is_available: bool
    d_is_available: bool
    e_is_available: bool


class ReservationSeat(BaseModel):
    id: int
    reservation_id: str
    seat: str
    created_at: datetime


class Reservation(BaseModel):
    id: str
    user_id: str
    schedule_id: str
    from_station_id: str
    to_station_id: str
    departure_at: str
    entry_token: str
    created_at: datetime


class ReservationQrImage(BaseModel):
    id: str
    reservation_id: str
    image: bytes


class Payment(BaseModel):
    id: int
    user_id: str
    reservation_id: str
    amount: int
    is_captured: bool
    is_refunded: bool
    created_at: datetime
    updated_at: datetime
