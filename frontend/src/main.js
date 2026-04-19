const API_BASE = 'http://127.0.0.1:8000/api';

const state = {
    game: null,
    mode: 'regular',
    selected: [],
    log: [],
    sizeInput: 9,
};

const app = document.querySelector('#app');

app.innerHTML = `
    <style>
        html, body, #app {
            height: 100%;
            overflow: hidden;
        }

        ::selection {
            background-color: #06b6d4;
            color: #0f172a;
        }
    </style>
    <main class="mx-auto flex h-screen w-[min(1280px,96vw)] flex-col overflow-hidden py-1.5">
        <header class="mb-2">
            <div class="flex justify-center">
                <img src="/logotransparent.png" alt="Qo" class="h-16 w-auto select-none" draggable="false" />
            </div>
        </header>

        <section class="min-h-0 flex-1 overflow-hidden grid gap-2 lg:grid-cols-[250px_minmax(460px,1fr)_280px] lg:items-stretch lg:justify-center">
            <aside class="min-h-0 overflow-auto rounded-xl border border-slate-800 bg-slate-900/85 p-3 lg:h-full">
                <h2 class="mb-3 text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">Game Controls</h2>

                <div class="flex h-full flex-col gap-4">
                    <div class="rounded-lg border border-slate-800 bg-slate-950/45 p-3">
                        <p id="turn-pill" class="mb-1 text-sm font-semibold text-slate-200"></p>
                        <p id="selection-hint" class="text-xs text-slate-400"></p>
                    </div>

                    <div class="space-y-2">
                        <p class="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">1. Setup</p>
                        <div class="grid grid-cols-[1fr_auto] gap-2">
                            <label for="board-size" class="col-span-2 text-xs text-slate-400">Board Size</label>
                            <input id="board-size" type="number" min="5" max="19" value="9" class="w-full rounded border border-slate-700 bg-slate-950 px-2 py-1.5 text-sm text-slate-100 outline-none" />
                            <button id="new-game" class="rounded border border-slate-700 bg-slate-800 px-3 py-1.5 text-xs font-medium text-slate-100">New Game</button>
                        </div>
                    </div>

                    <div class="space-y-2">
                        <p class="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">2. Select Move Type</p>
                        <div class="grid grid-cols-1 gap-2">
                            <button data-mode="regular" class="mode-btn rounded border border-slate-700 bg-cyan-700 px-3 py-2 text-xs font-medium text-white">Regular</button>
                            <button data-mode="superposition" class="mode-btn rounded border border-slate-700 bg-slate-800 px-3 py-2 text-xs font-medium text-slate-200">Superposition</button>
                            <button data-mode="entangle" class="mode-btn rounded border border-slate-700 bg-slate-800 px-3 py-2 text-xs font-medium text-slate-200">Entangle</button>
                            <button data-mode="collapse" class="mode-btn rounded border border-slate-700 bg-slate-800 px-3 py-2 text-xs font-medium text-slate-200">Collapse Set</button>
                        </div>
                    </div>

                    <div class="space-y-2">
                        <p class="text-[11px] font-semibold uppercase tracking-[0.14em] text-slate-500">3. Apply Action</p>
                        <div class="grid grid-cols-1 gap-2">
                            <button id="commit-quantum" class="rounded border border-cyan-700 bg-cyan-800 px-3 py-2 text-xs font-semibold text-cyan-100">Commit Quantum Move</button>
                            <div class="grid grid-cols-2 gap-2">
                                <button id="pass" class="rounded border border-slate-700 bg-slate-800 px-3 py-2 text-xs font-medium text-slate-100">Pass</button>
                                <button id="clear-selection" class="rounded border border-slate-700 bg-slate-800 px-3 py-2 text-xs font-medium text-slate-100">Clear</button>
                            </div>
                        </div>
                        <p class="px-1 text-[11px] text-slate-500">Game ends after two full pass rounds (4 consecutive passes).</p>
                        <p class="mt-auto px-1 text-xs text-slate-500">Tip: choose a mode, click intersections, then commit for quantum moves.</p>
                    </div>
                </div>
            </aside>

            <section class="min-h-0 rounded-xl border border-slate-800 bg-slate-900/85 p-3 lg:h-full">
                <div class="flex h-full flex-col gap-3">
                    <div id="live-counts" class="flex items-center justify-center gap-3"></div>
                    <div class="min-h-0 flex-1 flex items-center justify-center">
                        <div id="board" class="grid aspect-square h-full w-full bg-transparent"></div>
                    </div>
                </div>
            </section>

            <aside class="min-h-0 grid gap-2 lg:h-full lg:grid-rows-[minmax(0,0.9fr)_minmax(0,1fr)_minmax(0,1.25fr)]">
                <section class="rounded-xl border border-slate-800 bg-slate-900/85 p-4">
                    <h2 class="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">Legend</h2>
                    <div class="h-full overflow-auto pr-2">
                        <ul class="space-y-1 text-xs text-slate-300">
                            <li class="flex items-center gap-2"><span class="h-3 w-3 rounded-full bg-slate-900 ring-1 ring-slate-500"></span> Regular Black</li>
                            <li class="flex items-center gap-2"><span class="h-3 w-3 rounded-full bg-slate-100 ring-1 ring-slate-500"></span> Regular White</li>
                            <li class="flex items-center gap-2"><span class="h-3 w-3 rounded-full border-2 border-dashed border-cyan-300"></span> Superposition</li>
                            <li class="flex items-center gap-2"><span class="h-3 w-3 rounded-full border-2 border-pink-300 shadow-[0_0_0_2px_rgba(244,114,182,0.25)]"></span> Entanglement</li>
                            <li class="flex items-center gap-2"><span class="h-3 w-3 rounded-full border-2 border-emerald-300"></span> Border color = set</li>
                        </ul>
                    </div>
                </section>

                <section class="rounded-xl border border-slate-800 bg-slate-900/85 p-4">
                    <h2 class="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">Quantum Systems</h2>
                    <div id="systems" class="h-full space-y-1 overflow-auto pr-2 text-xs text-slate-300 [overflow-wrap:anywhere]"></div>
                </section>

                <section class="rounded-xl border border-slate-800 bg-slate-900/85 p-4">
                    <h2 class="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">Event Log</h2>
                    <div id="log-list" class="h-full space-y-1 overflow-auto pr-2 text-xs text-slate-300 [overflow-wrap:anywhere]"></div>
                </section>
            </aside>
        </section>

        <p class="mt-2 text-center text-xs text-slate-500">Made in Chapel Hill 🐏</p>

    <div id="winner-modal" class="fixed inset-0 z-50 hidden items-center justify-center bg-black/65 p-4">
      <div class="w-full max-w-sm rounded-xl border border-slate-700 bg-slate-900 p-5 text-center shadow-2xl">
        <h3 class="text-lg font-semibold text-slate-100">Game Over</h3>
        <p id="winner-text" class="mt-2 text-sm text-slate-300"></p>
        <p id="winner-score" class="mt-1 text-xs text-slate-400"></p>
        <div class="mt-4 flex justify-center gap-2">
          <button id="modal-restart" class="rounded border border-cyan-700 bg-cyan-800 px-4 py-1.5 text-xs font-semibold text-cyan-100">Restart</button>
        </div>
      </div>
    </div>
  </main>
`;

const boardEl = document.querySelector('#board');
const turnPillEl = document.querySelector('#turn-pill');
const logListEl = document.querySelector('#log-list');
const systemsEl = document.querySelector('#systems');
const selectionHintEl = document.querySelector('#selection-hint');
const liveCountsEl = document.querySelector('#live-counts');
const winnerModalEl = document.querySelector('#winner-modal');
const winnerTextEl = document.querySelector('#winner-text');
const winnerScoreEl = document.querySelector('#winner-score');

const modeButtons = [...document.querySelectorAll('.mode-btn')];
const sizeInput = document.querySelector('#board-size');
const newGameBtn = document.querySelector('#new-game');
const commitQuantumBtn = document.querySelector('#commit-quantum');
const passBtn = document.querySelector('#pass');
const clearSelectionBtn = document.querySelector('#clear-selection');
const modalRestartBtn = document.querySelector('#modal-restart');

function controlsLocked() {
    return Boolean(state.game?.gameOver);
}

function applyModeButtonStyles() {
    modeButtons.forEach((btn) => {
        const active = btn.dataset.mode === state.mode;
        btn.classList.toggle('bg-cyan-700', active);
        btn.classList.toggle('text-white', active);
        btn.classList.toggle('bg-slate-800', !active);
        btn.classList.toggle('text-slate-200', !active);
    });
}

modeButtons.forEach((btn) => {
    btn.addEventListener('click', () => {
        if (controlsLocked()) return;
        state.mode = btn.dataset.mode;
        state.selected = [];
        applyModeButtonStyles();
        render();
    });
});

sizeInput.addEventListener('change', (e) => {
    state.sizeInput = Number(e.target.value);
});

async function startNewGame(size) {
    const nextSize = Math.max(5, Math.min(19, size || 9));
    const data = await post('/new-game', { size: nextSize });
    applyData(data);
    winnerModalEl.classList.add('hidden');
    winnerModalEl.classList.remove('flex');
}

newGameBtn.addEventListener('click', async () => {
    await startNewGame(state.sizeInput || 9);
});

modalRestartBtn.addEventListener('click', async () => {
    const currentSize = state.game?.size || state.sizeInput || 9;
    await startNewGame(currentSize);
});

commitQuantumBtn.addEventListener('click', async () => {
    await commitQuantumSelection();
});

passBtn.addEventListener('click', async () => {
    if (controlsLocked()) return;
    const data = await post('/move/pass', {});
    applyData(data);
});

clearSelectionBtn.addEventListener('click', () => {
    if (controlsLocked()) return;
    state.selected = [];
    renderBoard();
    renderSelectionHint();
});

async function getState() {
    const res = await fetch(`${API_BASE}/state`);
    if (!res.ok) {
        throw new Error('Failed to load game state');
    }
    return res.json();
}

async function post(path, body) {
    const res = await fetch(`${API_BASE}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
    });
    const data = await res.json();
    if (!res.ok) {
        state.log.unshift(data.detail || 'Action failed');
        renderLog();
        return { state: data.state, events: [] };
    }
    return data;
}

function applyData(data) {
    state.game = data.state;
    state.selected = [];
    if (data.events?.length) {
        state.log = [...data.events.reverse(), ...state.log].slice(0, 120);
    }
    render();
}

function toKey(x, y) {
    return `${x},${y}`;
}

function isSelected(x, y) {
    return state.selected.some((p) => p.key === toKey(x, y));
}

async function commitQuantumSelection() {
    if (!state.game || state.game.gameOver) {
        return;
    }

    if (state.mode === 'regular') {
        state.log.unshift('Regular mode places immediately; no commit needed.');
        renderLog();
        return;
    }

    if (state.mode === 'superposition') {
        if (state.selected.length !== 2) {
            state.log.unshift('Superposition needs exactly 2 empty intersections.');
            renderLog();
            return;
        }
        const data = await post('/move/superposition', {
            positions: state.selected.map((p) => [p.x, p.y]),
        });
        applyData(data);
        return;
    }

    if (state.mode === 'entangle') {
        if (state.selected.length < 2 || state.selected.length > 3) {
            state.log.unshift('Entanglement needs 2 or 3 classical stones.');
            renderLog();
            return;
        }

        const hasOwnStone = state.selected.some((p) => {
            const cell = state.game.board[p.y]?.[p.x];
            return cell?.owner === state.game.currentPlayer;
        });
        if (!hasOwnStone) {
            state.log.unshift('Entanglement requires at least one of your stones.');
            renderLog();
            return;
        }

        const data = await post('/move/entangle', {
            positions: state.selected.map((p) => [p.x, p.y]),
        });
        applyData(data);
        return;
    }

    if (state.mode === 'collapse') {
        state.log.unshift('Collapse mode: click a superposition or entangled stone on the board.');
        renderLog();
        return;
    }
}

async function onCellClick(x, y) {
    if (!state.game || state.game.gameOver) {
        return;
    }

    const cell = state.game.board[y][x];

    if (state.mode === 'collapse') {
        if (cell.kind === 'superposition') {
            const data = await post('/move/collapse', { kind: 'superposition', systemId: cell.superpositionId });
            applyData(data);
            return;
        }

        if (cell.kind === 'entangled') {
            const data = await post('/move/collapse', { kind: 'entanglement', systemId: cell.entanglementId });
            applyData(data);
            return;
        }

        state.log.unshift('Collapse mode: click a superposition (S#) or entangled (E#) stone.');
        renderLog();
        return;
    }

    if (state.mode === 'regular') {
        const data = await post('/move/regular', { x, y });
        applyData(data);
        return;
    }

    if (state.mode === 'superposition') {
        if (cell.kind !== 'empty') {
            state.log.unshift('Superposition requires empty intersections.');
            renderLog();
            return;
        }

        if (isSelected(x, y)) {
            state.selected = state.selected.filter((p) => p.key !== toKey(x, y));
            renderBoard();
            renderSelectionHint();
            return;
        }

        if (state.selected.length >= 2) {
            state.log.unshift('Superposition can include only 2 positions.');
            renderLog();
            return;
        }

        state.selected.push({ key: toKey(x, y), x, y });
        renderBoard();
        renderSelectionHint();
        return;
    }

    if (state.mode === 'entangle') {
        if (cell.kind !== 'classical') {
            state.log.unshift('Entanglement can target only non-entangled classical stones.');
            renderLog();
            return;
        }

        if (isSelected(x, y)) {
            state.selected = state.selected.filter((p) => p.key !== toKey(x, y));
            renderBoard();
            renderSelectionHint();
            return;
        }

        if (state.selected.length < 3) {
            state.selected.push({ key: toKey(x, y), x, y });
        } else {
            state.log.unshift('Entanglement can include at most 3 stones.');
            renderLog();
            return;
        }

        renderBoard();
        renderSelectionHint();
    }
}

function setColor(kind, id) {
    const base = kind === 'superposition' ? 195 : 350;
    const hue = (base + (id * 47)) % 360;
    return `hsl(${hue} 92% 68%)`;
}

function renderSelectionHint() {
    if (!selectionHintEl) return;

    if (controlsLocked()) {
        selectionHintEl.textContent = 'Game ended. Restart to play again.';
        commitQuantumBtn.disabled = true;
        return;
    }

    if (state.mode === 'regular') {
        selectionHintEl.textContent = 'Regular: click to place.';
        commitQuantumBtn.disabled = true;
        return;
    }

    if (state.mode === 'collapse') {
        selectionHintEl.textContent = 'Collapse: click one superposition/entangled stone to collapse that set.';
        commitQuantumBtn.disabled = true;
        return;
    }

    if (state.mode === 'superposition') {
        selectionHintEl.textContent = `Superposition: ${state.selected.length}/2 selected.`;
        commitQuantumBtn.disabled = state.selected.length !== 2;
        return;
    }

    const ownCount = state.selected.filter((p) => {
        const cell = state.game?.board?.[p.y]?.[p.x];
        return cell?.owner === state.game?.currentPlayer;
    }).length;

    selectionHintEl.textContent = `Entangle: ${state.selected.length}/3 selected (min 2), own stones: ${ownCount}.`;
    commitQuantumBtn.disabled = state.selected.length < 2 || state.selected.length > 3 || ownCount < 1;
}

function baseStoneClasses(owner) {
    if (owner === 'B') {
        return 'bg-gradient-to-br from-slate-600 to-slate-900 ring-1 ring-slate-500/40 shadow-[0_2px_10px_rgba(0,0,0,0.55)]';
    }
    return 'bg-gradient-to-br from-white to-slate-200 ring-1 ring-slate-600/60 shadow-[0_2px_10px_rgba(8,16,24,0.45)]';
}

function renderBoard() {
    if (!state.game) return;

    const size = state.game.size;
    boardEl.innerHTML = '';
    boardEl.style.gridTemplateColumns = `repeat(${size}, minmax(0, 1fr))`;
    boardEl.style.gridTemplateRows = `repeat(${size}, minmax(0, 1fr))`;

    for (let y = 0; y < size; y += 1) {
        for (let x = 0; x < size; x += 1) {
            const cell = state.game.board[y][x];
            const item = document.createElement('button');
            item.type = 'button';
            item.className = 'relative h-full w-full bg-transparent p-0';
            item.dataset.x = String(x);
            item.dataset.y = String(y);
            item.disabled = controlsLocked();

            const hLine = document.createElement('span');
            hLine.className = 'pointer-events-none absolute top-1/2 h-px -translate-y-1/2 bg-slate-500/60';
            hLine.style.left = x === 0 ? '50%' : '0';
            hLine.style.width = x === 0 || x === size - 1 ? '50%' : '100%';
            if (x === size - 1) {
                hLine.style.left = '0';
            }
            item.appendChild(hLine);

            const vLine = document.createElement('span');
            vLine.className = 'pointer-events-none absolute left-1/2 w-px -translate-x-1/2 bg-slate-500/60';
            vLine.style.top = y === 0 ? '50%' : '0';
            vLine.style.height = y === 0 || y === size - 1 ? '50%' : '100%';
            if (y === size - 1) {
                vLine.style.top = '0';
            }
            item.appendChild(vLine);

            if (isSelected(x, y)) {
                hLine.className = 'pointer-events-none absolute top-1/2 h-[2px] -translate-y-1/2 bg-cyan-300/80';
                vLine.className = 'pointer-events-none absolute left-1/2 w-[2px] -translate-x-1/2 bg-cyan-300/80';
            }

            if (cell.kind !== 'empty') {
                const stone = document.createElement('span');
                stone.className = `pointer-events-none absolute left-1/2 top-1/2 aspect-square w-[72%] -translate-x-1/2 -translate-y-1/2 rounded-full ${baseStoneClasses(cell.owner)}`;

                if (cell.kind === 'superposition') {
                    const color = setColor('superposition', cell.superpositionId);
                    stone.style.border = `2px dashed ${color}`;
                    stone.style.boxShadow = 'inset 0 0 0 2px rgba(8,18,26,0.9)';

                    const core = document.createElement('span');
                    core.className = 'absolute left-1/2 top-1/2 h-[30%] w-[30%] -translate-x-1/2 -translate-y-1/2 rounded-full';
                    core.style.background = color;
                    core.style.opacity = '0.72';
                    stone.appendChild(core);
                }

                if (cell.kind === 'entangled') {
                    const color = setColor('entangled', cell.entanglementId);
                    stone.style.border = `2px solid ${color}`;
                    stone.style.boxShadow = `inset 0 0 0 2px rgba(8,18,26,0.9), 0 0 0 4px color-mix(in srgb, ${color} 26%, transparent)`;

                    const innerRing = document.createElement('span');
                    innerRing.className = 'absolute inset-[15%] rounded-full border';
                    innerRing.style.borderColor = color;
                    innerRing.style.opacity = '0.9';
                    stone.appendChild(innerRing);
                }

                item.appendChild(stone);
            }

            item.addEventListener('click', () => onCellClick(x, y));
            boardEl.appendChild(item);
        }
    }
}

function renderTurn() {
    if (!state.game) return;

    const live = state.game.liveCounts || { B: 0, W: 0 };
    liveCountsEl.innerHTML = `
        <div class="min-w-[120px] rounded-md border border-slate-700 bg-slate-900 px-3 py-1.5 text-center text-sm font-semibold text-slate-100">${live.B}</div>
        <div class="min-w-[120px] rounded-md border border-slate-300 bg-slate-100 px-3 py-1.5 text-center text-sm font-semibold text-slate-900">${live.W}</div>
    `;

    if (state.game.gameOver) {
        const score = state.game.score || { B: 0, W: 0 };
        turnPillEl.textContent = `Game Over | B ${score.B} : W ${score.W}`;

        let winnerText = 'Result: Draw';
        if (score.B > score.W) winnerText = 'Winner: Black';
        if (score.W > score.B) winnerText = 'Winner: White';
        winnerTextEl.textContent = winnerText;
        winnerScoreEl.textContent = `Final Score - Black ${score.B}, White ${score.W}`;
        winnerModalEl.classList.remove('hidden');
        winnerModalEl.classList.add('flex');
    } else {
        const playerLabel = state.game.currentPlayer === 'B' ? 'Black' : 'White';
        turnPillEl.textContent = `Turn: ${playerLabel} | Mode: ${state.mode}`;
        winnerModalEl.classList.add('hidden');
        winnerModalEl.classList.remove('flex');
    }
}

function renderLog() {
    logListEl.innerHTML = '';
    for (const entry of state.log.slice(0, 40)) {
        const line = document.createElement('p');
        line.className = 'text-xs leading-5 text-slate-300 break-words whitespace-normal';
        line.textContent = entry;
        logListEl.appendChild(line);
    }
}

function renderSystems() {
    if (!state.game) return;

    const chunks = [];

    for (const s of state.game.superpositions) {
        chunks.push(`<p><strong>S${s.id}</strong> (${s.owner}): [${s.positions.map((p) => `${p[0]},${p[1]}`).join('] & [')}]</p>`);
    }

    for (const e of state.game.entanglements) {
        chunks.push(`<p><strong>E${e.id}</strong> (${e.initiator}): ${e.positions.map((p, idx) => `[${p[0]},${p[1]}:${e.probabilities[idx]}]`).join(' ')}</p>`);
    }

    if (chunks.length === 0) {
        systemsEl.innerHTML = '<p class="text-xs text-slate-500">No active quantum systems.</p>';
        return;
    }

    systemsEl.innerHTML = chunks.join('');
}

function updateControlsDisabled() {
    const locked = controlsLocked();
    passBtn.disabled = locked;
    clearSelectionBtn.disabled = locked;
    if (locked) {
        commitQuantumBtn.disabled = true;
    }
    modeButtons.forEach((btn) => {
        btn.disabled = locked;
        btn.classList.toggle('opacity-50', locked);
    });
}

function render() {
    applyModeButtonStyles();
    renderTurn();
    renderBoard();
    renderSystems();
    renderSelectionHint();
    renderLog();
    updateControlsDisabled();
}

(async function bootstrap() {
    try {
        const data = await getState();
        applyData(data);
        state.log.unshift('Welcome to Qo.');
        renderLog();
    } catch (err) {
        state.log.unshift('Could not connect to backend at http://127.0.0.1:8000.');
        renderLog();
        console.error(err);
    }
})();
