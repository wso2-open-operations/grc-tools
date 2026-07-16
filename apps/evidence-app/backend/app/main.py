from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api.routes import products, frameworks, controls, evidence, submissions, agent, usage, me
from app.config import settings

app = FastAPI(title="Compliance Evidence Portal", version="1.0.0", redirect_slashes=False)

cors_origins = [o.strip() for o in settings.CORS_ORIGINS.split(",") if o.strip()]

app.add_middleware(
    CORSMiddleware,
    allow_origins=cors_origins,
    allow_methods=["*"],
    allow_headers=["*"],
)

# The unauthenticated GET /uploads/{filename} route that used to stream blobs
# straight out of private storage has been removed — evidence files are now
# served via short-lived signed Azure URLs generated at read time
# (app/storage/blob_storage.py:get_signed_url). See ADR 0003.

app.include_router(products.router, prefix="/api")
app.include_router(frameworks.router, prefix="/api")
app.include_router(controls.router, prefix="/api")
app.include_router(evidence.router, prefix="/api")
app.include_router(submissions.router, prefix="/api")
app.include_router(agent.router, prefix="/api")
app.include_router(usage.router, prefix="/api")
app.include_router(me.router, prefix="/api")


@app.get("/health")
def health_check():
    return {"status": "ok"}
