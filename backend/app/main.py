from __future__ import annotations

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from .engine import QoGame
from .schemas import (
    ApiResponse,
    EntangleMoveRequest,
    NewGameRequest,
    RegularMoveRequest,
    SuperpositionMoveRequest,
)

app = FastAPI(title="Qo Backend", version="0.1.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:5173", "http://127.0.0.1:5173"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

game = QoGame()


def _success(events: list[str]) -> ApiResponse:
    return ApiResponse(state=game.state(), events=events)


def _error(message: str, status_code: int = 400) -> JSONResponse:
    return JSONResponse(status_code=status_code, content={"detail": message, "state": game.state()})


@app.get("/api/state", response_model=ApiResponse)
def get_state() -> ApiResponse:
    return _success(events=[])


@app.post("/api/new-game", response_model=ApiResponse)
def new_game(payload: NewGameRequest) -> ApiResponse:
    game.reset(payload.size)
    return _success([f"Started a new {payload.size}x{payload.size} game."])


@app.post("/api/move/regular", response_model=ApiResponse)
def move_regular(payload: RegularMoveRequest):
    try:
        return _success(game.place_regular(payload.x, payload.y))
    except ValueError as exc:
        return _error(str(exc))


@app.post("/api/move/superposition", response_model=ApiResponse)
def move_superposition(payload: SuperpositionMoveRequest):
    try:
        return _success(game.place_superposition(payload.positions))
    except ValueError as exc:
        return _error(str(exc))


@app.post("/api/move/entangle", response_model=ApiResponse)
def move_entangle(payload: EntangleMoveRequest):
    try:
        return _success(game.entangle(payload.positions))
    except ValueError as exc:
        return _error(str(exc))


@app.post("/api/move/pass", response_model=ApiResponse)
def move_pass():
    try:
        return _success(game.pass_turn())
    except ValueError as exc:
        return _error(str(exc))
