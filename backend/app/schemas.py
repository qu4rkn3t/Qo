from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field


StoneColor = Literal["B", "W"]


class NewGameRequest(BaseModel):
    size: int = Field(default=9, ge=5, le=19)


class RegularMoveRequest(BaseModel):
    x: int
    y: int


class SuperpositionMoveRequest(BaseModel):
    positions: list[list[int]] = Field(min_length=2, max_length=2)


class EntangleMoveRequest(BaseModel):
    positions: list[list[int]] = Field(min_length=2, max_length=3)


class CollapseMoveRequest(BaseModel):
    kind: Literal["superposition", "entanglement"]
    systemId: int = Field(ge=1)


class ApiResponse(BaseModel):
    state: dict
    events: list[str] = Field(default_factory=list)
