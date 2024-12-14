from datetime import datetime
from enum import Enum

from sqlalchemy import text

from .models import Setting
from .sql import engine

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
