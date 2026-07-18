import os

from sqlalchemy import create_engine, text
from sqlalchemy.engine import make_url

connection_string = os.environ.get("DATABASE_URL", "")
if "sslmode=verify-full" not in connection_string:
    raise RuntimeError("DATABASE_URL must contain sslmode=verify-full")

url = make_url(connection_string).set(drivername="postgresql+psycopg")
engine = create_engine(
    url,
    connect_args={"connect_timeout": 10},
    pool_size=1,
    max_overflow=0,
)

try:
    with engine.connect() as connection:
        row = connection.execute(
            text(
                "SELECT current_user AS role_name, "
                "COALESCE((SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()), false) AS tls_active"
            )
        ).mappings().one()
        if not row["role_name"] or row["tls_active"] is not True:
            raise RuntimeError("SQLAlchemy connected without the expected TLS session metadata")
    print("PASS: SQLAlchemy connects through the gateway with verified TLS")
finally:
    engine.dispose()
