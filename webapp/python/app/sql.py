import os

import sqlalchemy

host = os.getenv("ISHOCON_DB_HOST", "127.0.0.1")
port = int(os.getenv("ISHOCON_DB_PORT", "3306"))
user = os.getenv("ISHOCON_DB_USER", "ishocon")
password = os.getenv("ISHOCON_DB_PASSWORD", "ishocon")
dbname = os.getenv("ISHOCON_DB_NAME", "ishocon3")

engine = sqlalchemy.create_engine(
    f"mysql+pymysql://{user}:{password}@{host}:{port}/{dbname}",
    pool_size=100,
    max_overflow=0
)
