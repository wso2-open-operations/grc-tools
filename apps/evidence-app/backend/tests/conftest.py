"""
Shared pytest fixtures for the backend test suite.

The app builds module-level state straight from the environment at import
time (`app/config.py`'s `Settings()`, `app/auth.py`'s Asgardeo userinfo URL,
`app/main.py`'s Azure `BlobServiceClient`), so every required setting must
already be in `os.environ` before `app.main` — or anything that imports it —
is imported for the first time anywhere in the test run. That's why the env
defaults below run before any `app.*` import in this file.

Tests exercise the FastAPI app over HTTP via `TestClient`, with two
dependencies overridden so no real Asgardeo or Azure account is needed:

- `get_current_user` -> a fake Engineer or Admin identity (`engineer_client`
  / `admin_client` fixtures below).
- `get_db` -> a session on a throwaway Postgres database, wrapped in a
  transaction that's rolled back after every test for per-test isolation
  (`db_session` fixture below).

Postgres was chosen over SQLite so Postgres-specific column types (e.g.
`DateTime(timezone=True)`, `JSON`) behave the same in tests as in production
— see issue #6/#7. A real Postgres must be reachable at `TEST_DATABASE_URL`
before running the suite, e.g.:

    docker run -d --name evidence-app-test-db \\
        -e POSTGRES_PASSWORD=postgres -e POSTGRES_USER=postgres \\
        -e POSTGRES_DB=evidence_app_test -p 5433:5432 postgres:16
"""
import os

TEST_DATABASE_URL = os.environ.get(
    "TEST_DATABASE_URL",
    "postgresql://postgres:postgres@localhost:5433/evidence_app_test",
)

os.environ.setdefault("DATABASE_URL", TEST_DATABASE_URL)
# A syntactically valid connection string (the well-known Azurite emulator
# credential) so `BlobServiceClient.from_connection_string` parses cleanly at
# import time. No network call happens at import — routes that actually touch
# blob storage are out of scope for this smoke-level harness.
os.environ.setdefault(
    "AZURE_STORAGE_CONNECTION_STRING",
    "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;"
    "AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;"
    "BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;",
)
# Explicit, even though app/config.py still defaults this to "gvgj" today —
# ticket #8 removes that default and makes the setting required, and this
# harness should not regress when that lands.
os.environ.setdefault("ASGARDEO_ORG", "test-org")

import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine, event
from sqlalchemy.orm import Session

from app.auth import User, get_current_user
from app.database import Base, get_db
from app.main import app

# Import every model so Base.metadata knows about all tables — mirrors the
# import list in alembic/env.py, which needs the same thing for autogenerate.
from app.models import (  # noqa: F401
    agent_task,
    control,
    evidence,
    evidence_file,
    framework,
    product,
    submission,
    usage_log,
)


@pytest.fixture(scope="session")
def db_engine():
    """Session-scoped engine against the throwaway test Postgres. Creates
    every table from the SQLAlchemy models once for the whole run and drops
    them at the end; per-test isolation is handled by `db_session` below."""
    engine = create_engine(TEST_DATABASE_URL)
    try:
        with engine.connect():
            pass
    except Exception as exc:
        pytest.exit(
            "Could not reach the throwaway test Postgres at "
            f"{TEST_DATABASE_URL!r} ({exc!r}).\n"
            "Start one, e.g.:\n"
            "  docker run -d --name evidence-app-test-db "
            "-e POSTGRES_PASSWORD=postgres -e POSTGRES_USER=postgres "
            "-e POSTGRES_DB=evidence_app_test -p 5433:5432 postgres:16",
            returncode=1,
        )

    Base.metadata.create_all(engine)
    try:
        yield engine
    finally:
        Base.metadata.drop_all(engine)
        engine.dispose()


@pytest.fixture()
def db_session(db_engine):
    """One Postgres session per test.

    The session is bound to a connection holding an outer transaction plus a
    SAVEPOINT, and both are rolled back at teardown — so nothing a test does
    is ever visible to another test, even though route handlers call
    `db.commit()` themselves (some, like evidence creation, call it more than
    once per request). This is the standard SQLAlchemy pattern for joining a
    Session to an external transaction in a test suite.
    """
    connection = db_engine.connect()
    outer_transaction = connection.begin()
    session = Session(bind=connection)

    nested = connection.begin_nested()

    @event.listens_for(session, "after_transaction_end")
    def _restart_savepoint(session, transaction):
        nonlocal nested
        if not nested.is_active:
            nested = connection.begin_nested()

    try:
        yield session
    finally:
        session.close()
        outer_transaction.rollback()
        connection.close()


@pytest.fixture()
def engineer_user() -> User:
    return User(email="engineer@example.com", role="engineer")


@pytest.fixture()
def admin_user() -> User:
    return User(email="admin@example.com", role="admin")


@pytest.fixture()
def client(db_session):
    """A TestClient with only `get_db` overridden — `get_current_user` is
    left wired to the real dependency, so requests without a Bearer token
    still get a real 401. Useful for auth-boundary tests; most tests want
    `engineer_client` or `admin_client` instead."""
    app.dependency_overrides[get_db] = lambda: db_session
    try:
        yield TestClient(app)
    finally:
        app.dependency_overrides.pop(get_db, None)


def _client_as(db_session, user: User):
    app.dependency_overrides[get_db] = lambda: db_session
    app.dependency_overrides[get_current_user] = lambda: user
    try:
        yield TestClient(app)
    finally:
        app.dependency_overrides.pop(get_db, None)
        app.dependency_overrides.pop(get_current_user, None)


@pytest.fixture()
def engineer_client(db_session, engineer_user):
    """A TestClient with `get_db` pointed at the isolated test session and
    `get_current_user` faked as an Engineer — no real Asgardeo call."""
    yield from _client_as(db_session, engineer_user)


@pytest.fixture()
def admin_client(db_session, admin_user):
    """Same as `engineer_client`, but faked as an Admin."""
    yield from _client_as(db_session, admin_user)
