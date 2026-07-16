"""
Every stored file reference (`"/uploads/{blob_name}"`) is converted to a
short-lived Azure SAS URL at read/serialization time — never stored that
way — and the old unauthenticated `GET /uploads/{filename}` route is gone
(see ADR 0003 / issue #9).

`conftest.py` sets `AZURE_STORAGE_CONNECTION_STRING` to a well-formed,
parseable account-key connection string (the standard Azurite emulator
credential), so `generate_blob_sas` can sign locally with no network call.
"""
from urllib.parse import parse_qs, urlparse

from app.models.agent_task import AgentTask
from app.models.evidence import Evidence
from app.models.evidence_file import EvidenceFile


def _assert_signed(url: str) -> None:
    """A signed URL is not the bare stored reference: it points straight at
    the blob and carries a SAS signature + expiry."""
    assert not url.startswith("/uploads/"), url
    query = parse_qs(urlparse(url).query)
    assert "sig" in query, url
    assert "se" in query, url


def test_evidence_response_file_url_is_signed_not_uploads_path(engineer_client, db_session):
    evidence = Evidence(
        title="Screenshot of control panel",
        file_name="abc123.png",
        file_url="/uploads/abc123.png",
        created_by="engineer@example.com",
    )
    db_session.add(evidence)
    db_session.commit()

    response = engineer_client.get(f"/api/evidence/{evidence.id}")

    assert response.status_code == 200
    _assert_signed(response.json()["file_url"])


def test_evidence_file_response_file_url_is_signed(engineer_client, db_session):
    evidence = Evidence(
        title="Screenshot of control panel",
        file_name="abc123.png",
        file_url="/uploads/abc123.png",
        created_by="engineer@example.com",
    )
    db_session.add(evidence)
    db_session.flush()
    db_session.add(
        EvidenceFile(
            evidence_id=evidence.id,
            file_name="def456.png",
            file_url="/uploads/def456.png",
            sort_order=0,
        )
    )
    db_session.commit()

    response = engineer_client.get(f"/api/evidence/{evidence.id}")

    assert response.status_code == 200
    files = response.json()["files"]
    assert len(files) == 1
    _assert_signed(files[0]["file_url"])
    # The two files point at different blobs, so their signed URLs must differ.
    assert files[0]["file_url"] != response.json()["file_url"]


def test_evidence_list_signs_every_row(engineer_client, db_session):
    db_session.add(
        Evidence(
            title="One",
            file_name="one.png",
            file_url="/uploads/one.png",
            created_by="engineer@example.com",
        )
    )
    db_session.commit()

    response = engineer_client.get("/api/evidence")

    assert response.status_code == 200
    body = response.json()
    assert len(body) == 1
    _assert_signed(body[0]["file_url"])


def test_agent_task_screenshot_payload_is_signed(engineer_client, db_session):
    task = AgentTask(
        user_email="engineer@example.com",
        prompt="capture the dashboard",
        status="completed",
        result={
            "status": "completed",
            "result": "done",
            "screenshots": [
                {"file_name": "shot1.png", "file_url": "/uploads/shot1.png", "subtask": "step 1"},
                {"file_name": "shot2.png", "file_url": "/uploads/shot2.png", "subtask": "step 2"},
            ],
        },
    )
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 200
    screenshots = response.json()["result"]["screenshots"]
    assert len(screenshots) == 2
    _assert_signed(screenshots[0]["file_url"])
    _assert_signed(screenshots[1]["file_url"])
    assert screenshots[0]["file_url"] != screenshots[1]["file_url"]
    # Non-file_url fields pass through untouched.
    assert screenshots[0]["subtask"] == "step 1"


def test_agent_task_list_signs_screenshots_too(engineer_client, db_session):
    task = AgentTask(
        user_email="engineer@example.com",
        prompt="capture the dashboard",
        status="completed",
        result={"status": "completed", "screenshots": [{"file_name": "a.png", "file_url": "/uploads/a.png"}]},
    )
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get("/api/agent/tasks")

    assert response.status_code == 200
    body = response.json()
    assert len(body) == 1
    _assert_signed(body[0]["result"]["screenshots"][0]["file_url"])


def test_agent_task_without_screenshots_is_unaffected(engineer_client, db_session):
    """A task with no result yet (still queued) must not blow up the signing
    validator — `result` is None."""
    task = AgentTask(user_email="engineer@example.com", prompt="do something", status="queued")
    db_session.add(task)
    db_session.commit()

    response = engineer_client.get(f"/api/agent/tasks/{task.id}")

    assert response.status_code == 200
    assert response.json()["result"] is None


def test_uploads_route_no_longer_exists(client):
    """The old unauthenticated proxy route is gone entirely — not just
    auth-gated, actually removed."""
    response = client.get("/uploads/anything.png")

    assert response.status_code == 404
