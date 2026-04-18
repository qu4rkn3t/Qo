# Qo

Qo is a local two-player Quantum Go variant with a Python backend and a Bun-powered JavaScript frontend.

## Stack

- Backend: FastAPI (Python), managed with uv
- Frontend: Vanilla JavaScript + Vite, managed with Bun

## Rules in Qo

Qo keeps the normal flow of Go turns and pass logic, and adds three quantum mechanics:

1. Superposition
- Place one stone into exactly two empty intersections in one turn.
- On measurement, each superposition collapses 50/50 into one classical position.

2. Entanglement
- Entangle 2 or 3 classical stones in one turn.
- Initiator must own at least one selected stone.
- Superposed stones cannot be entangled.
- On measurement, one stone in the set is sampled (equal probability), and the set collapses to that sampled stone's owner.

3. Tunneling
- Triggered whenever surrounded classical stones exist.
- Before resolving tunneling, all quantum systems are measured and collapsed.
- The engine picks random board destinations and maps them to surrounded stones.
- Tunnel success probability decreases with enemy barrier density between source and destination.
- Success: stone moves instantly to destination.
- Failure: stone is not removed; it flips and becomes an enemy stone.

Game end condition stays classic: two consecutive passes end the game, and area score decides the winner.

## Run the Backend (uv)

```bash
cd backend
uv sync
uv run uvicorn app.main:app --reload
```

Backend listens on `http://127.0.0.1:8000`.

## Run the Frontend (bun)

```bash
cd frontend
bun install
bun run dev
```

Frontend runs on `http://127.0.0.1:5173` and calls the backend API.

## Gameplay UI Notes

- `Regular` mode: click one intersection to place a classical stone.
- `Superposition` mode: click two empty intersections.
- `Entangle` mode: click 2 or 3 classical stones.
- `Measure All`: manually collapse active quantum systems.
- `Pass`: pass turn; two passes end the game.

Visual encoding:
- Classical stones are solid black/white.
- Superpositions use dashed cyan ring + `S{id}/{index}` marker.
- Entangled stones use red ring + `E{id}` marker.

## API Endpoints

- `GET /api/state`
- `POST /api/new-game` with `{ "size": 9 }`
- `POST /api/move/regular` with `{ "x": 3, "y": 3 }`
- `POST /api/move/superposition` with `{ "positions": [[1,1], [2,2]] }`
- `POST /api/move/entangle` with `{ "positions": [[1,1], [1,2]] }`
- `POST /api/move/pass` with `{}`
- `POST /api/measure` with `{}`
