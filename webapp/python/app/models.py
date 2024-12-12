from datetime import datetime

import pydantic


class BaseModel(pydantic.BaseModel):
    model_config = pydantic.ConfigDict(from_attributes=True)


class StationModel(BaseModel):
    id: str
    name: str

class SettingModel(BaseModel):
    initialized_at: datetime
