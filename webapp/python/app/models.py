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
    api_call_at: datetime | None
    created_at: datetime
