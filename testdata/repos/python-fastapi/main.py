from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI(title="FastAPI Example", version="1.0.0")


class HealthResponse(BaseModel):
    status: str


class MessageResponse(BaseModel):
    message: str


@app.get("/", response_model=MessageResponse)
async def root():
    return MessageResponse(message="Hello from FastAPI!")


@app.get("/health", response_model=HealthResponse)
async def health():
    return HealthResponse(status="healthy")


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
