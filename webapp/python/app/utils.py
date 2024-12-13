from datetime import datetime

def get_application_clock(initialized_at: datetime) -> str:
    time_passed = datetime.now() - initialized_at
    # この世界では1秒が10分に相当する
    # 24:00で世界が止まる
    hours = min(time_passed.seconds // 6, 24)
    minutes = time_passed.seconds % 6 * 10 if hours < 24 else 0
    return f"{hours:02d}:{minutes:02d}"
