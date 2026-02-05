# agentix/server.py

from fastapi import FastAPI
from fastapi.responses import JSONResponse

from .models import get_model, get_models
from .transforms import transform_ollama_tags_to_openai_engines

app = FastAPI()


class DummyArgs:
    """A dummy args class to simulate command-line arguments."""

    def __init__(self):
        self.debug = False
        self.model = None


@app.get("/v1/models")
async def list_models():
    return JSONResponse(content={"models": ["model-1", "model-2"]})


@app.post("/v1/completions")
async def create_completion():
    return JSONResponse(
        content={"id": "completion-id", "object": "text_completion", "choices": []}
    )


@app.post("/v1/edits")
async def create_edit():
    return JSONResponse(content={"id": "edit-id", "object": "edit", "choices": []})


@app.post("/v1/images/generations")
async def create_image():
    return JSONResponse(content={"id": "image-id", "object": "image", "data": []})


@app.post("/v1/audio/transcriptions")
async def create_transcription():
    return JSONResponse(
        content={"id": "transcription-id", "object": "transcription", "text": ""}
    )


@app.post("/v1/audio/translations")
async def create_translation():
    return JSONResponse(
        content={"id": "translation-id", "object": "translation", "text": ""}
    )


@app.post("/v1/files")
async def upload_file():
    return JSONResponse(content={"id": "file-id", "object": "file"})


@app.get("/v1/files")
async def list_files():
    return JSONResponse(content={"data": []})


@app.get("/v1/files/{file_id}")
async def retrieve_file(file_id: str):
    return JSONResponse(content={"id": file_id, "object": "file"})


@app.delete("/v1/files/{file_id}")
async def delete_file(file_id: str):
    return JSONResponse(content={"id": file_id, "object": "file", "deleted": True})


@app.post("/v1/fine-tunes")
async def create_fine_tune():
    return JSONResponse(content={"id": "fine-tune-id", "object": "fine_tune"})


@app.get("/v1/fine-tunes")
async def list_fine_tunes():
    return JSONResponse(content={"data": []})


@app.get("/v1/fine-tunes/{fine_tune_id}")
async def retrieve_fine_tune(fine_tune_id: str):
    return JSONResponse(content={"id": fine_tune_id, "object": "fine_tune"})


@app.post("/v1/fine-tunes/{fine_tune_id}/cancel")
async def cancel_fine_tune(fine_tune_id: str):
    return JSONResponse(
        content={"id": fine_tune_id, "object": "fine_tune", "status": "cancelled"}
    )


@app.get("/v1/engines")
async def list_engines():
    args = DummyArgs()  # Placeholder for args if needed
    return JSONResponse(
        content={"data": transform_ollama_tags_to_openai_engines(get_models(args))}
    )


@app.get("/v1/engines/{engine_id}")
async def retrieve_engine(engine_id: str):
    args = DummyArgs()  # Placeholder for args if needed
    engines = transform_ollama_tags_to_openai_engines(
        get_models(args), filter_tag=engine_id
    )
    engine = engines["data"][0] if engines["data"] else None
    if engine:
        return JSONResponse(content=engine)
    return JSONResponse(content={"error": "Engine not found"}, status_code=404)


def start_server(port: int):
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=port)
