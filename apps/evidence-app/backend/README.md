# Compliance Evidence Portal — Backend

FastAPI service that stores compliance evidence and coordinates the browser-automation
runner. Deployed on Choreo as a REST service (see `.choreo/component.yaml`).

## Stack

- **FastAPI** REST API (`app/main.py`)
- **PostgreSQL** (Neon in production) via SQLAlchemy + Alembic
- **Azure Blob Storage** for evidence files (required — there is no local fallback)
- **Asgardeo** OAuth2 for authentication (`app/auth.py`)

## Layout

```
app/
  main.py              FastAPI app, CORS, blob file serving, router registration
  config.py            Settings loaded from environment (.env)
  auth.py              Asgardeo bearer-token validation + dev header fallback
  rbac.py              require_admin / require_engineer_or_admin dependencies
  database.py          SQLAlchemy engine + session
  models/              ORM models
  schemas/             Pydantic request/response models
  api/routes/          One module per resource
  storage/             blob_storage.py — Azure Blob upload/delete
alembic/               Database migrations
```

## Local development

```bash
python3.11 -m venv venv
source venv/bin/activate
pip install -r requirements.txt

cp .env.example .env        # then fill in the values
alembic upgrade head
uvicorn app.main:app --reload --port 8000
```

Every request must carry a valid Asgardeo `Authorization: Bearer <token>` — the
same locally as in production. The web app obtains it via Asgardeo sign-in and
the Runner via its own Asgardeo login.

## Environment variables

See [`.env.example`](.env.example). All variables there are required except
`ADMIN_EMAILS` (may be empty) and the ones with defaults.

## Database migrations

```bash
alembic upgrade head                      # apply pending migrations
alembic revision --autogenerate -m "..."  # generate a migration from model changes
```

New models must be imported in `alembic/env.py` for autogenerate to detect them.
