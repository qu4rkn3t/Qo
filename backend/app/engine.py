from __future__ import annotations

from collections import deque
from dataclasses import dataclass, field
import random
from typing import Literal

from .quantum import sample_superposition_index, sample_weighted_index

StoneColor = Literal["B", "W"]


def opponent(color: StoneColor) -> StoneColor:
    return "W" if color == "B" else "B"


@dataclass
class SuperpositionSystem:
    system_id: int
    owner: StoneColor
    positions: tuple[tuple[int, int], tuple[int, int]]


@dataclass
class EntanglementSystem:
    system_id: int
    initiator: StoneColor
    positions: tuple[tuple[int, int], ...]
    probabilities: tuple[float, ...]


@dataclass
class QoGame:
    size: int = 9
    current_player: StoneColor = "B"
    classical: dict[tuple[int, int], StoneColor] = field(default_factory=dict)
    superpositions: dict[int, SuperpositionSystem] = field(default_factory=dict)
    entanglements: dict[int, EntanglementSystem] = field(default_factory=dict)
    consecutive_passes: int = 0
    game_over: bool = False
    next_superposition_id: int = 1
    next_entanglement_id: int = 1
    ko_forbidden_signature: tuple[tuple[int, int, StoneColor], ...] | None = None

    def reset(self, size: int) -> None:
        self.size = size
        self.current_player = "B"
        self.classical.clear()
        self.superpositions.clear()
        self.entanglements.clear()
        self.consecutive_passes = 0
        self.game_over = False
        self.next_superposition_id = 1
        self.next_entanglement_id = 1
        self.ko_forbidden_signature = None

    def _in_bounds(self, x: int, y: int) -> bool:
        return 0 <= x < self.size and 0 <= y < self.size

    def _neighbors(self, x: int, y: int) -> list[tuple[int, int]]:
        points: list[tuple[int, int]] = []
        if x > 0:
            points.append((x - 1, y))
        if x < self.size - 1:
            points.append((x + 1, y))
        if y > 0:
            points.append((x, y - 1))
        if y < self.size - 1:
            points.append((x, y + 1))
        return points

    def _position_occupied(self, pos: tuple[int, int]) -> bool:
        if pos in self.classical:
            return True
        for system in self.superpositions.values():
            if pos in system.positions:
                return True
        return False

    def _validate_turn(self) -> None:
        if self.game_over:
            raise ValueError("Game is over. Start a new game.")

    def place_regular(self, x: int, y: int) -> list[str]:
        self._validate_turn()
        if not self._in_bounds(x, y):
            raise ValueError("Move is out of bounds.")
        pos = (x, y)
        if self._position_occupied(pos):
            raise ValueError("Position is already occupied.")

        snapshot = self._snapshot_state()
        pre_move_signature = self._board_signature()
        acting_player = self.current_player

        try:
            self.classical[pos] = acting_player
            self.consecutive_passes = 0

            events = [f"{acting_player} placed a regular stone at ({x}, {y})."]

            events.extend(self._collapse_quantum_adjacent_to(pos))
            events.extend(
                self._resolve_turn_end(
                    trigger_positions={pos},
                    acting_player=acting_player,
                    pre_move_signature=pre_move_signature,
                    enforce_ko=True,
                )
            )

            if pos in self.classical:
                _, own_liberties = self._group_and_liberties(pos, self.classical)
                if not own_liberties and not self._surrounded_by_own_color(pos, acting_player):
                    raise ValueError("Suicide is not allowed.")
            return events
        except ValueError:
            self._restore_state(snapshot)
            raise

    def _surrounded_by_own_color(self, pos: tuple[int, int], color: StoneColor) -> bool:
        neighbors = self._neighbors(pos[0], pos[1])
        return all(self.classical.get(np) == color for np in neighbors)

    def place_superposition(self, positions: list[list[int]]) -> list[str]:
        self._validate_turn()
        normalized = tuple((int(p[0]), int(p[1])) for p in positions)
        if normalized[0] == normalized[1]:
            raise ValueError("Superposition requires two distinct points.")

        for x, y in normalized:
            if not self._in_bounds(x, y):
                raise ValueError("Superposition includes out-of-bounds point.")
            if self._position_occupied((x, y)):
                raise ValueError("Superposition point is already occupied.")

        snapshot = self._snapshot_state()
        pre_move_signature = self._board_signature()
        acting_player = self.current_player

        try:
            system_id = self.next_superposition_id
            self.next_superposition_id += 1
            self.superpositions[system_id] = SuperpositionSystem(
                system_id=system_id,
                owner=acting_player,
                positions=(normalized[0], normalized[1]),
            )
            self.consecutive_passes = 0

            events = [
                (
                    f"{acting_player} created superposition S{system_id} at "
                    f"({normalized[0][0]}, {normalized[0][1]}) and ({normalized[1][0]}, {normalized[1][1]})."
                )
            ]
            events.extend(
                self._resolve_turn_end(
                    trigger_positions=set(normalized),
                    acting_player=acting_player,
                    pre_move_signature=pre_move_signature,
                    enforce_ko=True,
                )
            )
            return events
        except ValueError:
            self._restore_state(snapshot)
            raise

    def entangle(self, positions: list[list[int]]) -> list[str]:
        self._validate_turn()
        if len(positions) < 2 or len(positions) > 3:
            raise ValueError("Entanglement requires 2 or 3 stones.")

        normalized = tuple((int(p[0]), int(p[1])) for p in positions)
        if len(set(normalized)) != len(normalized):
            raise ValueError("Entanglement points must be distinct.")

        owners: list[StoneColor] = []
        for pos in normalized:
            if not self._in_bounds(pos[0], pos[1]):
                raise ValueError("Entanglement includes out-of-bounds point.")
            if pos not in self.classical:
                raise ValueError("Entanglement can target only classical stones.")
            if self._position_in_entanglement(pos):
                raise ValueError("A selected stone is already entangled.")
            owners.append(self.classical[pos])

        if self.current_player not in owners:
            raise ValueError(
                f"Entanglement requires at least one of your stones ({self.current_player})."
            )

        snapshot = self._snapshot_state()
        pre_move_signature = self._board_signature()
        acting_player = self.current_player

        try:
            system_id = self.next_entanglement_id
            self.next_entanglement_id += 1
            p = round(1.0 / len(normalized), 4)
            probabilities = tuple([p] * len(normalized))
            self.entanglements[system_id] = EntanglementSystem(
                system_id=system_id,
                initiator=acting_player,
                positions=normalized,
                probabilities=probabilities,
            )
            self.consecutive_passes = 0

            events = [f"{acting_player} created entanglement E{system_id} with {len(normalized)} stones."]
            events.extend(
                self._resolve_turn_end(
                    trigger_positions=set(normalized),
                    acting_player=acting_player,
                    pre_move_signature=pre_move_signature,
                    enforce_ko=True,
                )
            )
            return events
        except ValueError:
            self._restore_state(snapshot)
            raise

    def collapse_one_system(self, kind: Literal["superposition", "entanglement"], system_id: int) -> list[str]:
        self._validate_turn()

        snapshot = self._snapshot_state()
        pre_move_signature = self._board_signature()
        acting_player = self.current_player

        try:
            self.consecutive_passes = 0

            if kind == "superposition":
                if system_id not in self.superpositions:
                    raise ValueError(f"Superposition S{system_id} does not exist.")
                trigger_positions = set(self.superpositions[system_id].positions)
                events = [f"{acting_player} forced collapse of S{system_id}."]
                events.extend(self._collapse_superpositions({system_id}))
            else:
                if system_id not in self.entanglements:
                    raise ValueError(f"Entanglement E{system_id} does not exist.")
                trigger_positions = set(self.entanglements[system_id].positions)
                events = [f"{acting_player} forced collapse of E{system_id}."]
                events.extend(self._collapse_entanglements({system_id}))

            events.extend(
                self._resolve_turn_end(
                    trigger_positions=trigger_positions,
                    acting_player=acting_player,
                    pre_move_signature=pre_move_signature,
                    enforce_ko=True,
                )
            )
            return events
        except ValueError:
            self._restore_state(snapshot)
            raise

    def pass_turn(self) -> list[str]:
        self._validate_turn()
        self.consecutive_passes += 1
        events = [f"{self.current_player} passed."]

        if self.consecutive_passes >= 4:
            collapse_events = self._collapse_all_quantum()
            events.extend(collapse_events)
            self.game_over = True
            score = self._compute_area_score()
            events.append(
                f"Game ended after two consecutive pass rounds. Score: B={score['B']}, W={score['W']}."
            )
            if score["B"] == score["W"]:
                events.append("The game is a draw.")
            else:
                winner = "B" if score["B"] > score["W"] else "W"
                events.append(f"Winner: {winner}.")
            return events

        self.ko_forbidden_signature = None
        self.current_player = opponent(self.current_player)
        return events

    def measure_all(self) -> list[str]:
        self._validate_turn()
        events: list[str] = []
        events.extend(self._collapse_all_quantum())
        return events

    def _resolve_turn_end(
        self,
        trigger_positions: set[tuple[int, int]] | None,
        acting_player: StoneColor,
        pre_move_signature: tuple[tuple[int, int, StoneColor], ...],
        enforce_ko: bool,
    ) -> list[str]:
        events: list[str] = []
        events.extend(self._trigger_tunneling_if_needed(trigger_positions, acting_player))

        final_signature = self._board_signature()
        if (
            enforce_ko
            and final_signature != pre_move_signature
            and self.ko_forbidden_signature is not None
            and final_signature == self.ko_forbidden_signature
        ):
            raise ValueError("Ko rule violation: this move repeats the previous board position.")

        self.ko_forbidden_signature = pre_move_signature
        self.current_player = opponent(self.current_player)
        return events

    def _snapshot_state(self) -> dict:
        return {
            "current_player": self.current_player,
            "classical": dict(self.classical),
            "superpositions": dict(self.superpositions),
            "entanglements": dict(self.entanglements),
            "consecutive_passes": self.consecutive_passes,
            "game_over": self.game_over,
            "next_superposition_id": self.next_superposition_id,
            "next_entanglement_id": self.next_entanglement_id,
            "ko_forbidden_signature": self.ko_forbidden_signature,
        }

    def _restore_state(self, snapshot: dict) -> None:
        self.current_player = snapshot["current_player"]
        self.classical = dict(snapshot["classical"])
        self.superpositions = dict(snapshot["superpositions"])
        self.entanglements = dict(snapshot["entanglements"])
        self.consecutive_passes = snapshot["consecutive_passes"]
        self.game_over = snapshot["game_over"]
        self.next_superposition_id = snapshot["next_superposition_id"]
        self.next_entanglement_id = snapshot["next_entanglement_id"]
        self.ko_forbidden_signature = snapshot["ko_forbidden_signature"]

    def _board_signature(self) -> tuple[tuple[int, int, StoneColor], ...]:
        return tuple(sorted((x, y, color) for (x, y), color in self.classical.items()))

    def _capture_groups_of_color(
        self,
        color: StoneColor,
        candidate_positions: set[tuple[int, int]] | None = None,
    ) -> list[tuple[int, int]]:
        removed: list[tuple[int, int]] = []
        visited: set[tuple[int, int]] = set()

        if candidate_positions is None:
            positions = [pos for pos, owner in self.classical.items() if owner == color]
        else:
            positions = [
                pos
                for pos in candidate_positions
                if self.classical.get(pos) == color
            ]

        for pos in positions:
            if pos in visited or pos not in self.classical or self.classical[pos] != color:
                continue
            group, liberties = self._group_and_liberties(pos, self.classical)
            visited.update(group)
            if liberties:
                continue
            for gp in group:
                if gp in self.classical:
                    del self.classical[gp]
            removed.extend(group)

        return removed

    def _remove_all_zero_liberty_groups(self) -> dict[StoneColor, int]:
        removed: dict[StoneColor, int] = {"B": 0, "W": 0}
        while True:
            captured_any = False
            for color in ("B", "W"):
                captured = self._capture_groups_of_color(color)
                if captured:
                    removed[color] += len(captured)
                    captured_any = True
            if not captured_any:
                break
        return removed

    def _position_in_entanglement(self, pos: tuple[int, int]) -> bool:
        for system in self.entanglements.values():
            if pos in system.positions:
                return True
        return False

    def _collapse_all_quantum(self) -> list[str]:
        events: list[str] = []
        events.extend(self._collapse_superpositions())
        events.extend(self._collapse_entanglements())
        return events

    def _collapse_quantum_adjacent_to(self, pos: tuple[int, int]) -> list[str]:
        neighbors = set(self._neighbors(pos[0], pos[1]))

        super_ids: set[int] = set()
        for system_id, system in self.superpositions.items():
            if any(p in neighbors for p in system.positions):
                super_ids.add(system_id)

        ent_ids: set[int] = set()
        for system_id, system in self.entanglements.items():
            if any(p in neighbors for p in system.positions):
                ent_ids.add(system_id)

        events: list[str] = []
        if super_ids or ent_ids:
            events.append(
                (
                    f"Regular placement at ({pos[0]}, {pos[1]}) triggered adjacent quantum collapse."
                )
            )
        events.extend(self._collapse_superpositions(super_ids))
        events.extend(self._collapse_entanglements(ent_ids))
        return events

    def _collapse_superpositions(self, selected_ids: set[int] | None = None) -> list[str]:
        events: list[str] = []
        ids = sorted(selected_ids) if selected_ids is not None else sorted(self.superpositions.keys())
        for system_id in ids:
            system = self.superpositions.pop(system_id, None)
            if system is None:
                continue
            idx = sample_superposition_index()
            chosen = system.positions[idx]
            self.classical[chosen] = system.owner
            events.append(
                (
                    f"Superposition S{system_id} collapsed to ({chosen[0]}, {chosen[1]}) "
                    f"for {system.owner}."
                )
            )
        return events

    def _collapse_entanglements(self, selected_ids: set[int] | None = None) -> list[str]:
        events: list[str] = []
        ids = sorted(selected_ids) if selected_ids is not None else sorted(self.entanglements.keys())
        for system_id in ids:
            system = self.entanglements.pop(system_id, None)
            if system is None:
                continue
            idx = sample_weighted_index(system.probabilities)
            original_colors = [self.classical.get(pos, system.initiator) for pos in system.positions]
            rotated_colors = [
                original_colors[(i - idx) % len(original_colors)]
                for i in range(len(original_colors))
            ]
            for i, pos in enumerate(system.positions):
                self.classical[pos] = rotated_colors[i]
            events.append(
                (
                    f"Entanglement E{system_id} collapsed via permutation step {idx}: "
                    f"stone colors were reassigned within the entangled set."
                )
            )
        return events

    def _group_and_liberties(
        self,
        start: tuple[int, int],
        board: dict[tuple[int, int], StoneColor],
    ) -> tuple[set[tuple[int, int]], set[tuple[int, int]]]:
        color = board[start]
        q: deque[tuple[int, int]] = deque([start])
        visited = {start}
        group: set[tuple[int, int]] = set()
        liberties: set[tuple[int, int]] = set()

        while q:
            x, y = q.popleft()
            group.add((x, y))
            for nx, ny in self._neighbors(x, y):
                np = (nx, ny)
                if np not in board:
                    liberties.add(np)
                elif board[np] == color and np not in visited:
                    visited.add(np)
                    q.append(np)

        return group, liberties

    def _find_surrounded_stones(self, candidate_positions: set[tuple[int, int]] | None = None) -> list[tuple[int, int]]:
        surrounded: list[tuple[int, int]] = []
        seen: set[tuple[int, int]] = set()

        if candidate_positions is None:
            positions = list(self.classical.keys())
        else:
            positions = [pos for pos in candidate_positions if pos in self.classical]

        for pos in positions:
            if pos in seen:
                continue
            group, liberties = self._group_and_liberties(pos, self.classical)
            seen.update(group)
            if len(liberties) == 0:
                surrounded.extend(group)

        return surrounded

    def _sample_line_points(self, src: tuple[int, int], dst: tuple[int, int]) -> list[tuple[int, int]]:
        x0, y0 = src
        x1, y1 = dst
        dx = x1 - x0
        dy = y1 - y0
        steps = max(abs(dx), abs(dy))
        if steps <= 1:
            return []

        points: list[tuple[int, int]] = []
        for i in range(1, steps):
            t = i / steps
            x = round(x0 + dx * t)
            y = round(y0 + dy * t)
            points.append((x, y))
        return points

    def _tunnel_probability(
        self,
        stone_pos: tuple[int, int],
        destination: tuple[int, int],
        color: StoneColor,
    ) -> float:
        line_points = self._sample_line_points(stone_pos, destination)
        if not line_points:
            return 0.35

        enemy = opponent(color)
        barrier_count = sum(1 for p in line_points if self.classical.get(p) == enemy)
        steps = max(abs(destination[0] - stone_pos[0]), abs(destination[1] - stone_pos[1]))
        density = barrier_count / max(1, steps)

        base = 0.35 - (0.25 * density)
        if destination in self.classical and destination != stone_pos:
            base -= 0.15
        return max(0.03, min(0.45, base))

    def _random_destinations(self, k: int) -> list[tuple[int, int]]:
        all_points = [(x, y) for y in range(self.size) for x in range(self.size)]
        if k <= len(all_points):
            return random.sample(all_points, k)
        return [random.choice(all_points) for _ in range(k)]

    def _trigger_tunneling_if_needed(
        self,
        trigger_positions: set[tuple[int, int]] | None,
        acting_player: StoneColor,
    ) -> list[str]:
        events: list[str] = []
        if not trigger_positions:
            return events

        candidate_positions: set[tuple[int, int]] = set()
        for x, y in trigger_positions:
            if (x, y) in self.classical:
                candidate_positions.add((x, y))
            candidate_positions.update(self._neighbors(x, y))

        target_color = opponent(acting_player)
        candidate_positions = {
            pos
            for pos in candidate_positions
            if self.classical.get(pos) == target_color
        }
        if not candidate_positions:
            return events

        surrounded = self._find_surrounded_stones(candidate_positions)
        if not surrounded:
            return events

        if self.superpositions or self.entanglements:
            events.extend(self._collapse_all_quantum())
            candidate_positions = {
                pos
                for pos in candidate_positions
                if self.classical.get(pos) == target_color
            }
            if not candidate_positions:
                events.append("Tunneling canceled after collapse: no surrounded stones remained.")
                return events
            surrounded = self._find_surrounded_stones(candidate_positions)
            if not surrounded:
                events.append("Tunneling canceled after collapse: no surrounded stones remained.")
                return events

        k = len(surrounded)
        destinations = self._random_destinations(k)
        random.shuffle(destinations)
        events.append(f"Tunneling activated for {k} surrounded stones.")

        for stone_pos, destination in zip(surrounded, destinations):
            if stone_pos not in self.classical:
                continue
            color = self.classical[stone_pos]
            enemy = opponent(color)
            probability = self._tunnel_probability(stone_pos, destination, color)
            success = random.random() < probability and destination not in self.classical

            if success:
                del self.classical[stone_pos]
                self.classical[destination] = color
                events.append(
                    (
                        f"Stone at ({stone_pos[0]}, {stone_pos[1]}) tunneled to "
                        f"({destination[0]}, {destination[1]}) [p={probability:.2f}]."
                    )
                )
            else:
                self.classical[stone_pos] = enemy
                events.append(
                    (
                        f"Stone at ({stone_pos[0]}, {stone_pos[1]}) failed tunneling "
                        f"toward ({destination[0]}, {destination[1]}) and became {enemy} "
                        f"[p={probability:.2f}]."
                    )
                )

        return events

    def _compute_area_score(self) -> dict[StoneColor, int]:
        score: dict[StoneColor, int] = {"B": 0, "W": 0}

        for owner in self.classical.values():
            score[owner] += 1

        visited_empty: set[tuple[int, int]] = set()

        for y in range(self.size):
            for x in range(self.size):
                p = (x, y)
                if p in self.classical or p in visited_empty:
                    continue

                q: deque[tuple[int, int]] = deque([p])
                empty_region: set[tuple[int, int]] = set()
                bordering: set[StoneColor] = set()
                visited_empty.add(p)

                while q:
                    cx, cy = q.popleft()
                    cp = (cx, cy)
                    empty_region.add(cp)
                    for nx, ny in self._neighbors(cx, cy):
                        np = (nx, ny)
                        if np in self.classical:
                            bordering.add(self.classical[np])
                        elif np not in visited_empty:
                            visited_empty.add(np)
                            q.append(np)

                if len(bordering) == 1:
                    owner = next(iter(bordering))
                    score[owner] += len(empty_region)

        return score

    def _serialize_cell(self, x: int, y: int) -> dict:
        pos = (x, y)

        if pos in self.classical:
            entangled_id = None
            for system_id, system in self.entanglements.items():
                if pos in system.positions:
                    entangled_id = system_id
                    break
            if entangled_id is not None:
                return {
                    "kind": "entangled",
                    "owner": self.classical[pos],
                    "entanglementId": entangled_id,
                }
            return {"kind": "classical", "owner": self.classical[pos]}

        for system_id, system in self.superpositions.items():
            if pos in system.positions:
                idx = 0 if pos == system.positions[0] else 1
                return {
                    "kind": "superposition",
                    "owner": system.owner,
                    "superpositionId": system_id,
                    "pairIndex": idx,
                }

        return {"kind": "empty"}

    def _entanglement_summary(self) -> list[dict]:
        output: list[dict] = []
        for system_id, system in sorted(self.entanglements.items()):
            output.append(
                {
                    "id": system_id,
                    "initiator": system.initiator,
                    "positions": [[x, y] for (x, y) in system.positions],
                    "probabilities": list(system.probabilities),
                }
            )
        return output

    def _superposition_summary(self) -> list[dict]:
        output: list[dict] = []
        for system_id, system in sorted(self.superpositions.items()):
            output.append(
                {
                    "id": system_id,
                    "owner": system.owner,
                    "positions": [[x, y] for (x, y) in system.positions],
                }
            )
        return output

    def state(self) -> dict:
        board: list[list[dict]] = []
        for y in range(self.size):
            row: list[dict] = []
            for x in range(self.size):
                row.append(self._serialize_cell(x, y))
            board.append(row)

        score = self._compute_area_score() if self.game_over else None
        live_counts: dict[StoneColor, int] = {"B": 0, "W": 0}
        for owner in self.classical.values():
            live_counts[owner] += 1
        return {
            "size": self.size,
            "currentPlayer": self.current_player,
            "gameOver": self.game_over,
            "consecutivePasses": self.consecutive_passes,
            "board": board,
            "superpositions": self._superposition_summary(),
            "entanglements": self._entanglement_summary(),
            "liveCounts": live_counts,
            "score": score,
        }
