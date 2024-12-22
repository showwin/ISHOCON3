# -- bcrypt.hashpw("admin".encode(), "$2b$12$Lh9gc29tkPwGw0TPwTqjoO".encode()).decode()
# -- bcrypt.hashpw("ishocon".encode(), "$2b$12$W2VwGdhCXQt4ef6Zbrxnke".encode()).decode()
# insert into users (`id`, `name`, `hashed_password`, salt, is_admin, global_payment_token) values
# ("72b4f686-d283-4b63-b8bb-1e1fa7529c11", "admin", "$2b$12$Lh9gc29tkPwGw0TPwTqjoORtwJMvoJAXaXmurqNAcveQnSbHXkf8K", "$2b$12$Lh9gc29tkPwGw0TPwTqjoO", 1, "96d230c6-add1-498e-ac58-95212c9e869e"),
# ("aea4e2dc-3fb2-42eb-8070-97cf0c129a41", "ishocon", "$2b$12$W2VwGdhCXQt4ef6Zbrxnke4VO6PRVD0uZU5GAmVNjMp88CIT6hbI.", "$2b$12$W2VwGdhCXQt4ef6Zbrxnke", 0, "362f462c-1cb8-4054-890a-944c2b2437ef");

import bcrypt
from ulid import ULID

import os

import sqlalchemy
from sqlalchemy import text


host = os.getenv("ISHOCON_DB_HOST", "127.0.0.1")
port = int(os.getenv("ISHOCON_DB_PORT", "3306"))
user = os.getenv("ISHOCON_DB_USER", "ishocon")
password = os.getenv("ISHOCON_DB_PASSWORD", "ishocon")
dbname = os.getenv("ISHOCON_DB_NAME", "ishocon3")

engine = sqlalchemy.create_engine(
    f"mysql+pymysql://{user}:{password}@{host}:{port}/{dbname}"
)


user_count = 1000

with engine.begin() as conn:
  with open('users.csv', 'w') as f:
      f.write('name,password,global_payment_token\n')
      for i in range(user_count):
          name = "user" + str(i+ 1)
          user_id = str(ULID())
          password = str(ULID())
          global_payment_token = str(ULID())
          encoded_salt = bcrypt.gensalt()
          salt = encoded_salt.decode()
          hashed_password = bcrypt.hashpw(password.encode(), encoded_salt).decode()
          f.write(f'{name},{password},{global_payment_token}\n')

          conn.execute(
              text("insert into users (`id`, `name`, `hashed_password`, salt, is_admin, global_payment_token) values (:id, :name, :hashed_password, :salt, 0, :global_payment_token)"),
              {
                "id": user_id,
                "name": name,
                "hashed_password": hashed_password,
                "salt": salt,
                "global_payment_token": global_payment_token
              }
          )
