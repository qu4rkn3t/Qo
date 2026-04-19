package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)



var BoardSize = 9

type StoneType int

const (
	StEmpty    StoneType = 0
	StClassic  StoneType = 1
	StSuper    StoneType = 2
	StEntangle StoneType = 3
)

type Pos struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

type Cell struct {
	Owner   int       `json:"owner"`    // 0=empty, 1=black, 2=white
	SType   StoneType `json:"stype"`    // stone type
	GroupID int       `json:"group_id"` // -1 if not in quantum group
}

type SuperGroup struct {
	ID       int `json:"id"`
	Owner    int `json:"owner"`
	Pos1     Pos `json:"pos1"`
	Pos2     Pos `json:"pos2"`
	ColorIdx int `json:"color_idx"`
}

type EntGroup struct {
	ID       int   `json:"id"`
	Stones   []Pos `json:"stones"`
	Owners   []int `json:"owners"`
	ColorIdx int   `json:"color_idx"`
}

type TunnelResult struct {
	Stone   Pos     `json:"stone"`
	Target  Pos     `json:"target"`
	Prob    float64 `json:"prob"`
	Success bool    `json:"success"`
}

type GameEvent struct {
	EType   string      `json:"etype"`
	Msg     string      `json:"msg"`
	Details interface{} `json:"details,omitempty"`
}

type Game struct {
	Board                [][]Cell      `json:"board"`
	Size                 int           `json:"size"`
	Turn                 int           `json:"turn"` // 1=black, 2=white
	SuperGroups          []*SuperGroup `json:"super_groups"`
	EntGroups            []*EntGroup   `json:"ent_groups"`
	Passes               int           `json:"passes"`
	GameOver             bool          `json:"game_over"`
	BlackScore           float64       `json:"black_score"`
	WhiteScore           float64       `json:"white_score"`
	Winner               int           `json:"winner"`
	WinMsg               string        `json:"win_msg"`
	Events               []GameEvent   `json:"events"`
	NextGID              int           `json:"next_gid"`
	NextColorIdx         int           `json:"next_color_idx"`
	MoveCount            int           `json:"move_count"`
	BlackCaptures        int           `json:"black_captures"`
	WhiteCaptures        int           `json:"white_captures"`
	KoForbiddenSignature string        `json:"-"` // Track board state for Ko rule
}

var (
	game *Game
	mu   sync.Mutex
	rng  = rand.New(rand.NewSource(time.Now().UnixNano()))
)

var adjDirs = []Pos{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

// ─── Board State & Ko Rule ───────────────────────────────────────────────────

func (g *Game) boardSignature() string {
	// Create a string representing the current board state for Ko rule
	type cellKey struct{ r, c, owner int }
	var cells []cellKey
	for r := 0; r < g.Size; r++ {
		for c := 0; c < g.Size; c++ {
			// Only include classical stones (exclude superpositions)
			owner := g.ownerAt(r, c)
			if owner != 0 {
				cells = append(cells, cellKey{r, c, owner})
			}
		}
	}
	sig := ""
	for _, ck := range cells {
		sig += fmt.Sprintf("(%d,%d,%d)", ck.r, ck.c, ck.owner)
	}
	return sig
}

// ─── Initialisation ──────────────────────────────────────────────────────────

func newGame() *Game {
	b := make([][]Cell, BoardSize)
	for i := range b {
		b[i] = make([]Cell, BoardSize)
		for j := range b[i] {
			b[i][j] = Cell{GroupID: -1}
		}
	}
	return &Game{
		Board:   b,
		Size:    BoardSize,
		Turn:    1,
		NextGID: 1,
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func opp(p int) int {
	if p == 1 {
		return 2
	}
	return 1
}

func inBounds(r, c int) bool {
	return r >= 0 && r < BoardSize && c >= 0 && c < BoardSize
}

func iAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (g *Game) floodGroup(r, c int) []Pos {
	owner := g.ownerAt(r, c)
	if owner == 0 {
		return nil
	}
	seen := map[Pos]bool{}
	var group []Pos
	stack := []Pos{{r, c}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[p] {
			continue
		}
		seen[p] = true
		group = append(group, p)
		for _, d := range adjDirs {
			np := Pos{p.Row + d.Row, p.Col + d.Col}
			if inBounds(np.Row, np.Col) && g.ownerAt(np.Row, np.Col) == owner && !seen[np] {
				stack = append(stack, np)
			}
		}
	}
	return group
}

func (g *Game) libertyCount(r, c int) int {
	owner := g.ownerAt(r, c)
	if owner == 0 {
		return 0
	}
	seen := map[Pos]bool{}
	libSet := map[Pos]bool{}
	stack := []Pos{{r, c}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[p] {
			continue
		}
		seen[p] = true
		for _, d := range adjDirs {
			np := Pos{p.Row + d.Row, p.Col + d.Col}
			if !inBounds(np.Row, np.Col) {
				continue
			}
			v := g.ownerAt(np.Row, np.Col)
			if v == 0 {
				libSet[np] = true
			} else if v == owner {
				if !seen[np] {
					stack = append(stack, np)
				}
			}
		}
	}
	return len(libSet)
}

// Bresenham's line between two points (excluding endpoints)
func bresenhamPath(from, to Pos) []Pos {
	var pts []Pos
	r0, c0 := from.Row, from.Col
	r1, c1 := to.Row, to.Col
	dr := iAbs(r1 - r0)
	dc := iAbs(c1 - c0)
	sr, sc := 1, 1
	if r0 > r1 {
		sr = -1
	}
	if c0 > c1 {
		sc = -1
	}
	e := dr - dc
	for {
		e2 := 2 * e
		if e2 > -dc {
			e -= dc
			r0 += sr
		}
		if e2 < dr {
			e += dr
			c0 += sc
		}
		if r0 == r1 && c0 == c1 {
			break
		}
		pts = append(pts, Pos{r0, c0})
	}
	return pts
}

// ─── Quantum Group Management ─────────────────────────────────────────────────

func (g *Game) removeSuperGroup(id int) {
	for i, sg := range g.SuperGroups {
		if sg.ID == id {
			g.SuperGroups = append(g.SuperGroups[:i], g.SuperGroups[i+1:]...)
			return
		}
	}
}

func (g *Game) removeStoneFromEntGroup(p Pos, gid int) {
	for _, eg := range g.EntGroups {
		if eg.ID != gid {
			continue
		}
		for j, sp := range eg.Stones {
			if sp == p {
				eg.Stones = append(eg.Stones[:j], eg.Stones[j+1:]...)
				eg.Owners = append(eg.Owners[:j], eg.Owners[j+1:]...)
				return
			}
		}
	}
}

func (g *Game) removeEntGroup(id int) {
	for i, eg := range g.EntGroups {
		if eg.ID == id {
			g.EntGroups = append(g.EntGroups[:i], g.EntGroups[i+1:]...)
			return
		}
	}
}

func (g *Game) clearCell(p Pos) {
	gid := g.Board[p.Row][p.Col].GroupID
	stype := g.Board[p.Row][p.Col].SType
	if gid >= 0 {
		switch stype {
		case StSuper:
			g.removeSuperGroup(gid)
		case StEntangle:
			g.removeStoneFromEntGroup(p, gid)
		}
	}
	g.Board[p.Row][p.Col] = Cell{GroupID: -1}
}

func (g *Game) getNeighbors(r, c int) []Pos {
	var neighbors []Pos
	if r > 0 {
		neighbors = append(neighbors, Pos{r - 1, c})
	}
	if r < g.Size-1 {
		neighbors = append(neighbors, Pos{r + 1, c})
	}
	if c > 0 {
		neighbors = append(neighbors, Pos{r, c - 1})
	}
	if c < g.Size-1 {
		neighbors = append(neighbors, Pos{r, c + 1})
	}
	return neighbors
}

// ownerAt returns the classical owner at a board position, treating
// superposition and entangled stones as empty (owner 0). This mirrors the Python
// engine where only classical stones contribute to captures/liberties.
func (g *Game) ownerAt(r, c int) int {
	if !inBounds(r, c) {
		return 0
	}
	cell := g.Board[r][c]
	if cell.SType == StSuper || cell.SType == StEntangle {
		return 0
	}
	return cell.Owner
}

func (g *Game) collapseAdjacentQuantum(pos Pos) {
	neighbors := g.getNeighbors(pos.Row, pos.Col)
	neighborSet := map[Pos]bool{}
	for _, n := range neighbors {
		neighborSet[n] = true
	}

	superToCollapse := map[int]bool{}
	for _, sg := range g.SuperGroups {
		if neighborSet[sg.Pos1] || neighborSet[sg.Pos2] {
			superToCollapse[sg.ID] = true
		}
	}

	entToCollapse := map[int]bool{}
	for _, eg := range g.EntGroups {
		for _, p := range eg.Stones {
			if neighborSet[p] {
				entToCollapse[eg.ID] = true
				break
			}
		}
	}

	if len(superToCollapse) > 0 || len(entToCollapse) > 0 {
		g.Events = append(g.Events, GameEvent{
			EType: "quantum_collapse_adjacent",
			Msg:   fmt.Sprintf("Regular placement at (%d,%d) triggered adjacent quantum collapse.", pos.Row, pos.Col),
		})
		g.collapseSuperpositions(superToCollapse)
		g.collapseEntanglements(entToCollapse)
	}
}

// ─── Quantum Group Management ─────────────────────────────────────────────────

// ─── Quantum Mechanics ────────────────────────────────────────────────────────

func (g *Game) collapseAll() {
	superIDs := map[int]bool{}
	for _, sg := range g.SuperGroups {
		superIDs[sg.ID] = true
	}
	entIDs := map[int]bool{}
	for _, eg := range g.EntGroups {
		entIDs[eg.ID] = true
	}
	g.collapseSuperpositions(superIDs)
	g.collapseEntanglements(entIDs)
}

func (g *Game) collapseSuperpositions(ids map[int]bool) {
	for sysID := range ids {
		sg, found := findSuperGroup(g.SuperGroups, sysID)
		if !found {
			continue
		}
		chosen := rng.Intn(2)
		var keep, remove Pos
		if chosen == 0 {
			keep, remove = sg.Pos1, sg.Pos2
		} else {
			keep, remove = sg.Pos2, sg.Pos1
		}
		g.Board[keep.Row][keep.Col] = Cell{Owner: sg.Owner, SType: StClassic, GroupID: -1}
		g.Board[remove.Row][remove.Col] = Cell{GroupID: -1}
		g.Events = append(g.Events, GameEvent{
			EType: "superpos_collapse",
			Msg:   fmt.Sprintf("Superposition collapsed → stone survives at (%d,%d)", keep.Row, keep.Col),
		})
		g.removeSuperGroup(sysID)
	}
}

func (g *Game) collapseEntanglements(ids map[int]bool) {
	for sysID := range ids {
		eg, found := findEntGroup(g.EntGroups, sysID)
		if !found {
			continue
		}
		n := len(eg.Stones)
		if n == 0 {
			continue
		}

		// Select random index via weighted probability (equal probability for each)
		idx := rng.Intn(n)
		log.Printf("collapseEntanglements: group %d has %d stones, selected permutation index %d", sysID, n, idx)

		// Get original colors at each position (treat superpositions as empty)
		originalColors := make([]int, n)
		for i, p := range eg.Stones {
			if g.ownerAt(p.Row, p.Col) != 0 {
				originalColors[i] = g.ownerAt(p.Row, p.Col)
			} else {
				originalColors[i] = eg.Owners[i]
			}
		}

		// Apply cyclic permutation: rotated_colors[(i - idx) % n]
		rotatedColors := make([]int, n)
		for i := 0; i < n; i++ {
			rotatedColors[i] = originalColors[(i-idx+n)%n]
		}

		// Place rotated colors back
		blackCount := 0
		whiteCount := 0
		for i, p := range eg.Stones {
			g.Board[p.Row][p.Col] = Cell{Owner: rotatedColors[i], SType: StClassic, GroupID: -1}
			if rotatedColors[i] == 1 {
				blackCount++
			} else if rotatedColors[i] == 2 {
				whiteCount++
			}
		}
		log.Printf("collapseEntanglements: group %d collapsed to %d black, %d white stones", sysID, blackCount, whiteCount)
		g.Events = append(g.Events, GameEvent{
			EType: "entangle_collapse",
			Msg:   fmt.Sprintf("Entanglement collapsed via permutation step %d: stone colors reassigned.", idx),
		})
		g.removeEntGroup(sysID)
	}
}

func findSuperGroup(groups []*SuperGroup, id int) (*SuperGroup, bool) {
	for _, sg := range groups {
		if sg.ID == id {
			return sg, true
		}
	}
	return nil, false
}

func findEntGroup(groups []*EntGroup, id int) (*EntGroup, bool) {
	for _, eg := range groups {
		if eg.ID == id {
			return eg, true
		}
	}
	return nil, false
}

func (g *Game) collapseAllAndLog() []GameEvent {
	var events []GameEvent
	prevEventCount := len(g.Events)
	g.collapseAll()
	// Return only the new events added during collapse
	if len(g.Events) > prevEventCount {
		events = g.Events[prevEventCount:]
		g.Events = g.Events[:prevEventCount] // Reset to add them back in proper order
	}
	return events
}

func (g *Game) tunnelProb(stone, target Pos, enemy int) float64 {
	// Calculate distance-based barrier density
	dx := target.Col - stone.Col
	dy := target.Row - stone.Row
	steps := iAbs(dx)
	if iAbs(dy) > steps {
		steps = iAbs(dy)
	}

	if steps <= 1 {
		// Even short hops should fail more often than succeed
		return 0.35
	}

	// Sample points along line (Bresenham-like)
	barrierCount := 0
	for i := 1; i < steps; i++ {
		t := float64(i) / float64(steps)
		x := stone.Col + int(math.Round(float64(dx)*t))
		y := stone.Row + int(math.Round(float64(dy)*t))
		if inBounds(y, x) && g.ownerAt(y, x) == enemy {
			barrierCount++
		}
	}

	density := float64(barrierCount) / float64(steps)
	base := 0.35 - (0.25 * density)

	// Reduce probability if destination is occupied (classical occupancy only)
	if target.Row < len(g.Board) && target.Col < len(g.Board[0]) {
		if g.ownerAt(target.Row, target.Col) != 0 && target != stone {
			base -= 0.15
		}
	}

	// Clamp to [0.03, 0.45]
	if base < 0.03 {
		base = 0.03
	}
	if base > 0.45 {
		base = 0.45
	}
	return base
}

func (g *Game) triggerTunneling(group []Pos, capturingPlayer int) {
	tunneler := opp(capturingPlayer)

	// Must collapse all quantum states first
	if len(g.SuperGroups) > 0 || len(g.EntGroups) > 0 {
		g.Events = append(g.Events, GameEvent{
			EType: "quantum_collapse_trigger",
			Msg:   "⚡ Tunneling triggered — collapsing all quantum states!",
		})
		g.collapseAll()
		// After collapse, re-check whether the group is still surrounded
		// (quantum collapse may have changed adjacencies)
		stillTrapped := false
		if len(group) > 0 {
			p := group[0]
			if g.ownerAt(p.Row, p.Col) == tunneler {
				if g.libertyCount(p.Row, p.Col) == 0 {
					stillTrapped = true
				}
			}
		}
		if !stillTrapped {
			g.Events = append(g.Events, GameEvent{
				EType: "tunneling_aborted",
				Msg:   "Tunneling aborted — collapse freed the trapped stones.",
			})
			return
		}
	}

	// Collect available empty positions (excluding group positions)
	groupSet := map[Pos]bool{}
	for _, p := range group {
		groupSet[p] = true
	}
	var empty []Pos
	for r := 0; r < g.Size; r++ {
		for c := 0; c < g.Size; c++ {
			p := Pos{r, c}
			if g.ownerAt(r, c) == 0 && !groupSet[p] {
				empty = append(empty, p)
			}
		}
	}

	k := len(group)
	var results []TunnelResult

	if len(empty) == 0 {
		// No escape possible — classical Go: remove (capture) the group
		for _, p := range group {
			g.Board[p.Row][p.Col] = Cell{GroupID: -1}
		}
		if capturingPlayer == 1 {
			g.BlackCaptures += k
			log.Printf("triggerTunneling: BlackCaptures += %d -> %d (classical capture)", k, g.BlackCaptures)
		} else {
			g.WhiteCaptures += k
			log.Printf("triggerTunneling: WhiteCaptures += %d -> %d (classical capture)", k, g.WhiteCaptures)
		}
	} else {
		// Assign k random escape targets (shuffle and wrap)
		shuffledEmpty := make([]Pos, len(empty))
		copy(shuffledEmpty, empty)
		rng.Shuffle(len(shuffledEmpty), func(i, j int) { shuffledEmpty[i], shuffledEmpty[j] = shuffledEmpty[j], shuffledEmpty[i] })

		targets := make([]Pos, k)
		for i := 0; i < k; i++ {
			targets[i] = shuffledEmpty[i%len(shuffledEmpty)]
		}

		// Randomise which stone gets which target
		shuffledGroup := make([]Pos, k)
		copy(shuffledGroup, group)
		rng.Shuffle(k, func(i, j int) { shuffledGroup[i], shuffledGroup[j] = shuffledGroup[j], shuffledGroup[i] })

		// Attempt tunnels — clear originals first
		for _, p := range shuffledGroup {
			g.Board[p.Row][p.Col] = Cell{GroupID: -1}
		}

		usedTargets := map[Pos]bool{}
		for i, stone := range shuffledGroup {
			target := targets[i]
			prob := g.tunnelProb(stone, target, capturingPlayer)
			success := rng.Float64() < prob

			if success && !usedTargets[target] && g.ownerAt(target.Row, target.Col) == 0 {
				g.Board[target.Row][target.Col] = Cell{Owner: tunneler, SType: StClassic, GroupID: -1}
				usedTargets[target] = true
				log.Printf("triggerTunneling: stone %v tunneled to %v as owner=%d", stone, target, tunneler)
			} else {
				success = false
				// Becomes enemy stone at original location
				g.Board[stone.Row][stone.Col] = Cell{Owner: capturingPlayer, SType: StClassic, GroupID: -1}
				if capturingPlayer == 1 {
					g.BlackCaptures++
					log.Printf("triggerTunneling: stone %v failed -> owner=%d; BlackCaptures++ -> %d", stone, capturingPlayer, g.BlackCaptures)
				} else {
					g.WhiteCaptures++
					log.Printf("triggerTunneling: stone %v failed -> owner=%d; WhiteCaptures++ -> %d", stone, capturingPlayer, g.WhiteCaptures)
				}
			}

			results = append(results, TunnelResult{Stone: stone, Target: target, Prob: prob, Success: success})
		}
	}

	escaped := 0
	for _, r := range results {
		if r.Success {
			escaped++
		}
	}
	g.Events = append(g.Events, GameEvent{
		EType:   "tunneling",
		Msg:     fmt.Sprintf("🌀 Quantum tunneling: %d/%d stone(s) escaped!", escaped, k),
		Details: results,
	})
}

// ─── Capture Logic ────────────────────────────────────────────────────────────

func (g *Game) checkCaptures(placedR, placedC, player int) {
	enemy := opp(player)
	seen := map[Pos]bool{}
	for _, d := range adjDirs {
		nr, nc := placedR+d.Row, placedC+d.Col
		// Treat superpositions as empty when checking for classical captures
		if g.ownerAt(nr, nc) != enemy {
			continue
		}
		p := Pos{nr, nc}
		if seen[p] {
			continue
		}
		group := g.floodGroup(nr, nc)
		for _, gp := range group {
			seen[gp] = true
		}
		if g.libertyCount(nr, nc) > 0 {
			continue
		}
		// Group has no liberties — check stone types
		allClassical := true
		for _, gp := range group {
			// Both superpositions and entangled stones are quantum and should not tunnel
			if g.Board[gp.Row][gp.Col].SType == StSuper || g.Board[gp.Row][gp.Col].SType == StEntangle {
				allClassical = false
				break
			}
		}
		if allClassical {
			log.Printf("checkCaptures: triggering tunneling for classical group of size %d (player=%d)", len(group), player)
			g.triggerTunneling(group, player)
		} else {
			// Quantum stones: clear them
			log.Printf("checkCaptures: clearing quantum group of size %d captured by player %d", len(group), player)
			for _, gp := range group {
				g.clearCell(gp)
			}
			if player == 1 {
				g.BlackCaptures += len(group)
				log.Printf("checkCaptures: BlackCaptures += %d -> %d", len(group), g.BlackCaptures)
			} else {
				g.WhiteCaptures += len(group)
				log.Printf("checkCaptures: WhiteCaptures += %d -> %d", len(group), g.WhiteCaptures)
			}
		}
	}
}

func (g *Game) checkPostCollapseCaptures() {
	seen := map[Pos]bool{}
	for r := 0; r < g.Size; r++ {
		for c := 0; c < g.Size; c++ {
			p := Pos{r, c}
			if seen[p] || g.ownerAt(r, c) == 0 {
				continue
			}
			group := g.floodGroup(r, c)
			for _, gp := range group {
				seen[gp] = true
			}
			libCount := g.libertyCount(r, c)
			log.Printf("checkPostCollapseCaptures: pos (%d,%d) owner=%d libertyCount=%d", r, c, g.ownerAt(r, c), libCount)
			if libCount == 0 {
				owner := g.ownerAt(r, c)
				cap := opp(owner)
				log.Printf("checkPostCollapseCaptures: clearing group of %d stones (owner=%d, captured by=%d)", len(group), owner, cap)
				for _, gp := range group {
					g.Board[gp.Row][gp.Col] = Cell{GroupID: -1}
				}
				if cap == 1 {
					g.BlackCaptures += len(group)
					log.Printf("checkPostCollapseCaptures: BlackCaptures += %d -> %d", len(group), g.BlackCaptures)
				} else {
					g.WhiteCaptures += len(group)
					log.Printf("checkPostCollapseCaptures: WhiteCaptures += %d -> %d", len(group), g.WhiteCaptures)
				}
			}
		}
	}
	log.Printf("checkPostCollapseCaptures complete: black_captures=%d white_captures=%d", g.BlackCaptures, g.WhiteCaptures)
}

// ─── Move Functions ───────────────────────────────────────────────────────────

func (g *Game) placeClassical(pos Pos) error {
	if !inBounds(pos.Row, pos.Col) {
		return fmt.Errorf("out of bounds")
	}
	if g.Board[pos.Row][pos.Col].Owner != 0 {
		return fmt.Errorf("position already occupied")
	}
	preSignature := g.boardSignature()
	g.Board[pos.Row][pos.Col] = Cell{Owner: g.Turn, SType: StClassic, GroupID: -1}
	g.collapseAdjacentQuantum(pos)
	g.checkCaptures(pos.Row, pos.Col, g.Turn)
	// Suicide check
	if g.Board[pos.Row][pos.Col].Owner == g.Turn && g.libertyCount(pos.Row, pos.Col) == 0 {
		g.Board[pos.Row][pos.Col] = Cell{GroupID: -1}
		return fmt.Errorf("suicide move not allowed")
	}
	// Ko rule check
	postSignature := g.boardSignature()
	if postSignature == g.KoForbiddenSignature && postSignature != preSignature {
		g.Board[pos.Row][pos.Col] = Cell{GroupID: -1}
		return fmt.Errorf("ko rule violation")
	}
	g.KoForbiddenSignature = preSignature
	g.Events = append(g.Events, GameEvent{
		EType: "classical",
		Msg:   fmt.Sprintf("Stone placed at (%d,%d)", pos.Row, pos.Col),
	})
	return nil
}

func (g *Game) placeSuperpos(p1, p2 Pos) error {
	if !inBounds(p1.Row, p1.Col) || !inBounds(p2.Row, p2.Col) {
		return fmt.Errorf("out of bounds")
	}
	if p1 == p2 {
		return fmt.Errorf("positions must differ")
	}
	if g.Board[p1.Row][p1.Col].Owner != 0 {
		return fmt.Errorf("position 1 is occupied")
	}
	if g.Board[p2.Row][p2.Col].Owner != 0 {
		return fmt.Errorf("position 2 is occupied")
	}
	preSignature := g.boardSignature()
	gid := g.NextGID
	g.NextGID++
	ci := g.NextColorIdx % 8
	g.NextColorIdx++
	sg := &SuperGroup{ID: gid, Owner: g.Turn, Pos1: p1, Pos2: p2, ColorIdx: ci}
	g.SuperGroups = append(g.SuperGroups, sg)
	g.Board[p1.Row][p1.Col] = Cell{Owner: g.Turn, SType: StSuper, GroupID: gid}
	g.Board[p2.Row][p2.Col] = Cell{Owner: g.Turn, SType: StSuper, GroupID: gid}
	g.checkCaptures(p1.Row, p1.Col, g.Turn)
	g.checkCaptures(p2.Row, p2.Col, g.Turn)
	g.KoForbiddenSignature = preSignature
	g.Events = append(g.Events, GameEvent{
		EType: "superpos",
		Msg:   fmt.Sprintf("Superposition placed at (%d,%d) ↔ (%d,%d)", p1.Row, p1.Col, p2.Row, p2.Col),
	})
	return nil
}

func (g *Game) entangle(positions []Pos) error {
	if len(positions) < 2 || len(positions) > 3 {
		return fmt.Errorf("entanglement requires 2–3 stones")
	}
	seen := map[Pos]bool{}
	for _, p := range positions {
		if !inBounds(p.Row, p.Col) {
			return fmt.Errorf("position out of bounds")
		}
		if seen[p] {
			return fmt.Errorf("duplicate positions")
		}
		seen[p] = true
		c := g.Board[p.Row][p.Col]
		if c.Owner == 0 {
			return fmt.Errorf("no stone at (%d,%d)", p.Row, p.Col)
		}
		if c.SType != StClassic {
			return fmt.Errorf("only classical stones can be entangled")
		}
	}
	ownsOne := false
	for _, p := range positions {
		if g.Board[p.Row][p.Col].Owner == g.Turn {
			ownsOne = true
			break
		}
	}
	if !ownsOne {
		return fmt.Errorf("must own at least one stone in the entanglement")
	}

	preSignature := g.boardSignature()
	gid := g.NextGID
	g.NextGID++
	ci := g.NextColorIdx % 8
	g.NextColorIdx++
	owners := make([]int, len(positions))
	stones := make([]Pos, len(positions))
	for i, p := range positions {
		stones[i] = p
		owners[i] = g.Board[p.Row][p.Col].Owner
	}
	eg := &EntGroup{ID: gid, Stones: stones, Owners: owners, ColorIdx: ci}
	g.EntGroups = append(g.EntGroups, eg)
	for _, p := range positions {
		g.Board[p.Row][p.Col].SType = StEntangle
		g.Board[p.Row][p.Col].GroupID = gid
	}
	g.checkCaptures(positions[0].Row, positions[0].Col, g.Turn)
	g.KoForbiddenSignature = preSignature
	g.Events = append(g.Events, GameEvent{
		EType: "entangle",
		Msg:   fmt.Sprintf("%d stones entangled (group #%d)", len(positions), gid),
	})
	return nil
}

// ─── Scoring ──────────────────────────────────────────────────────────────────

func (g *Game) calcScore() (float64, float64) {
	visited := make([][]bool, g.Size)
	for i := range visited {
		visited[i] = make([]bool, g.Size)
	}
	var bTerritory, wTerritory float64
	for r := 0; r < g.Size; r++ {
		for c := 0; c < g.Size; c++ {
			if visited[r][c] || g.Board[r][c].Owner != 0 {
				continue
			}
			var region []Pos
			borders := map[int]bool{}
			stack := []Pos{{r, c}}
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if visited[p.Row][p.Col] {
					continue
				}
				visited[p.Row][p.Col] = true
				region = append(region, p)
				for _, d := range adjDirs {
					np := Pos{p.Row + d.Row, p.Col + d.Col}
					if !inBounds(np.Row, np.Col) {
						continue
					}
					if g.Board[np.Row][np.Col].Owner == 0 && !visited[np.Row][np.Col] {
						stack = append(stack, np)
					} else if g.Board[np.Row][np.Col].Owner != 0 {
						borders[g.Board[np.Row][np.Col].Owner] = true
					}
				}
			}
			if len(borders) == 1 {
				for owner := range borders {
					if owner == 1 {
						bTerritory += float64(len(region))
					} else {
						wTerritory += float64(len(region))
					}
				}
			}
		}
	}
	var bs, ws float64
	for r := 0; r < g.Size; r++ {
		for c := 0; c < g.Size; c++ {
			switch g.Board[r][c].Owner {
			case 1:
				bs++
			case 2:
				ws++
			}
		}
	}
	return bTerritory + bs, wTerritory + ws
}

// ─── HTTP Handlers ────────────────────────────────────────────────────────────

type MoveReq struct {
	MType string `json:"mtype"` // classical|superpos|entangle|measure|pass
	Pos   []Pos  `json:"pos"`
}

type Resp struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	State *Game  `json:"state"`
}

func playerName(p int) string {
	if p == 1 {
		return "Black"
	}
	return "White"
}

func handleMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req MoveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	mu.Lock()
	defer mu.Unlock()

	game.Events = nil
	resp := Resp{State: game}

	if game.GameOver {
		resp.Error = "game is over"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	var err error
	advance := false

	switch req.MType {
	case "classical":
		if len(req.Pos) != 1 {
			resp.Error = "need exactly 1 position"
			break
		}
		err = game.placeClassical(req.Pos[0])
		if err == nil {
			advance = true
		}

	case "superpos":
		if len(req.Pos) != 2 {
			resp.Error = "need exactly 2 positions"
			break
		}
		err = game.placeSuperpos(req.Pos[0], req.Pos[1])
		if err == nil {
			advance = true
		}

	case "entangle":
		if len(req.Pos) < 2 || len(req.Pos) > 3 {
			resp.Error = "need 2–3 positions"
			break
		}
		err = game.entangle(req.Pos)
		if err == nil {
			advance = true
		}

	case "measure":
		game.collapseAll()
		game.checkPostCollapseCaptures()
		game.Events = append(game.Events, GameEvent{
			EType: "measure",
			Msg:   playerName(game.Turn) + " triggered measurement — all quantum states collapsed.",
		})
		advance = true

	case "measure_group":
		if len(req.Pos) != 1 {
			resp.Error = "need exactly 1 position"
			break
		}
		pos := req.Pos[0]
		if !inBounds(pos.Row, pos.Col) {
			resp.Error = "out of bounds"
			break
		}
		cell := game.Board[pos.Row][pos.Col]
		if cell.SType == StClassic || cell.GroupID < 0 {
			resp.Error = "not a quantum stone"
			break
		}
		gid := cell.GroupID
		if cell.SType == StSuper {
			for _, sg := range game.SuperGroups {
				if sg.ID == gid {
					chosen := rng.Intn(2)
					var keep, remove Pos
					if chosen == 0 {
						keep, remove = sg.Pos1, sg.Pos2
					} else {
						keep, remove = sg.Pos2, sg.Pos1
					}
					game.Board[keep.Row][keep.Col] = Cell{Owner: sg.Owner, SType: StClassic, GroupID: -1}
					game.Board[remove.Row][remove.Col] = Cell{GroupID: -1}
					game.Events = append(game.Events, GameEvent{
						EType: "superpos_collapse",
						Msg:   fmt.Sprintf("Superposition collapsed → stone survives at (%d,%d)", keep.Row, keep.Col),
					})
					game.removeSuperGroup(gid)
					break
				}
			}
		} else if cell.SType == StEntangle {
			for _, eg := range game.EntGroups {
				if eg.ID == gid {
					n := len(eg.Stones)
					winner := rng.Intn(n)
					survivorPos := eg.Stones[winner]
					survivorOwner := eg.Owners[winner]
					for i, p := range eg.Stones {
						if i == winner {
							game.Board[p.Row][p.Col] = Cell{Owner: survivorOwner, SType: StClassic, GroupID: -1}
						} else {
							game.Board[p.Row][p.Col] = Cell{GroupID: -1}
						}
					}
					game.Events = append(game.Events, GameEvent{
						EType: "entangle_collapse",
						Msg:   fmt.Sprintf("Entanglement collapsed → stone at (%d,%d) survived", survivorPos.Row, survivorPos.Col),
					})
					game.removeEntGroup(gid)
					break
				}
			}
		}
		game.checkPostCollapseCaptures()
		game.Events = append(game.Events, GameEvent{
			EType: "measure_group",
			Msg:   playerName(game.Turn) + " measured a quantum group.",
		})
		advance = true

	case "pass":
		// Passing should NOT trigger tunneling/capture logic.
		game.Passes++
		game.Events = append(game.Events, GameEvent{
			EType: "pass",
			Msg:   playerName(game.Turn) + " passes.",
		})
		if game.Passes >= 2 {
			log.Printf("handleMove(pass): Passes >= 2, triggering game end and collapse")
			// 1. Collapse all quantum states before final scoring
			collapseEvents := game.collapseAllAndLog()
			game.Events = append(game.Events, collapseEvents...)
			log.Printf("handleMove(pass): after collapse, black_captures=%d white_captures=%d", game.BlackCaptures, game.WhiteCaptures)

			// 2. Remove (not flip) all groups with zero liberties, as in normal Go
			game.checkPostCollapseCaptures()
			log.Printf("handleMove(pass): after checkPostCollapseCaptures, black_captures=%d white_captures=%d", game.BlackCaptures, game.WhiteCaptures)

			// 3. Score the board
			b, wh := game.calcScore()
			game.BlackScore = b
			game.WhiteScore = wh
			game.GameOver = true
			if b > wh {
				game.Winner = 1
				game.WinMsg = fmt.Sprintf("Black wins! %.1f – %.1f", b, wh)
			} else if wh > b {
				game.Winner = 2
				game.WinMsg = fmt.Sprintf("White wins! %.1f – %.1f", wh, b)
			} else {
				game.WinMsg = "Draw!"
			}
			game.Events = append(game.Events, GameEvent{EType: "game_over", Msg: game.WinMsg})
		} else {
			game.KoForbiddenSignature = ""
			game.Turn = opp(game.Turn)
			game.MoveCount++
		}
		resp.OK = true

	default:
		resp.Error = "unknown move type"
	}

	if err != nil {
		resp.Error = err.Error()
	} else if advance && req.MType != "pass" {
		game.Passes = 0
		game.MoveCount++
		game.Turn = opp(game.Turn)
		resp.OK = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleState(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(game)
}

func handleReset(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	game = newGame()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(game)
}

type SetBoardSizeReq struct {
	Size int `json:"size"`
}

func handleSetBoardSize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req SetBoardSizeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.Size != 5 && req.Size != 9 && req.Size != 13 && req.Size != 19 {
		http.Error(w, "invalid size", 400)
		return
	}
	mu.Lock()
	BoardSize = req.Size
	game = newGame()
	mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(game)
}

// Debug handler to set predefined board patterns for reproduction/testing.
func handleDebugSetPattern(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	type Req struct {
		Pattern string `json:"pattern"`
	}
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	mu.Lock()
	defer mu.Unlock()

	switch req.Pattern {
	case "center-empty-white":
		// Fill board with white classical stones and leave center empty.
		for r := 0; r < game.Size; r++ {
			for c := 0; c < game.Size; c++ {
				game.Board[r][c] = Cell{Owner: 2, SType: StClassic, GroupID: -1}
			}
		}
		center := game.Size / 2
		game.Board[center][center] = Cell{GroupID: -1}
		game.SuperGroups = nil
		game.EntGroups = nil
		game.Passes = 0
		game.MoveCount = 0
		game.BlackCaptures = 0
		game.WhiteCaptures = 0
		game.Turn = 1
		game.KoForbiddenSignature = ""
		game.Events = nil
		json.NewEncoder(w).Encode(game)
		return
	case "repro-flip":
		// Create a large entanglement covering the board (except center),
		// with alternating owners in the entanglement owners array.
		gid := game.NextGID
		game.NextGID++
		var positions []Pos
		for r := 0; r < game.Size; r++ {
			for c := 0; c < game.Size; c++ {
				if r == game.Size/2 && c == game.Size/2 {
					continue
				}
				positions = append(positions, Pos{r, c})
			}
		}
		n := len(positions)
		owners := make([]int, n)
		for i := 0; i < n; i++ {
			if i%2 == 0 {
				owners[i] = 1
			} else {
				owners[i] = 2
			}
		}
		eg := &EntGroup{ID: gid, Stones: positions, Owners: owners, ColorIdx: 0}
		game.EntGroups = []*EntGroup{eg}
		// Set board cells to entangled but ownerless so collapse uses eg.Owners.
		for _, p := range positions {
			game.Board[p.Row][p.Col] = Cell{Owner: 0, SType: StEntangle, GroupID: gid}
		}
		// Ensure center is empty.
		center := game.Size / 2
		game.Board[center][center] = Cell{GroupID: -1}
		// Set passes so next pass triggers collapse.
		game.Passes = 1
		game.Turn = 1
		game.SuperGroups = nil
		game.BlackCaptures = 0
		game.WhiteCaptures = 0
		game.Events = nil
		json.NewEncoder(w).Encode(game)
		return
	default:
		http.Error(w, "unknown pattern", 400)
		return
	}
}

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(200)
			return
		}
		h(w, r)
	}
}

func killProcessOnPort(port string) {
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, ":"+port) && strings.Contains(line, "LISTENING") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				pid := fields[4]
				exec.Command("taskkill", "/PID", pid, "/F").Run()
			}
		}
	}
}

func main() {
	game = newGame()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", cors(handleState))
	mux.HandleFunc("/api/move", cors(handleMove))
	mux.HandleFunc("/api/reset", cors(handleReset))
	mux.HandleFunc("/api/setBoardSize", cors(handleSetBoardSize))
	// Debug endpoint for setting reproducible test patterns
	mux.HandleFunc("/api/debugSetPattern", cors(handleDebugSetPattern))
	mux.Handle("/", http.FileServer(http.Dir(".")))
	killProcessOnPort("8080")
	log.Println("▶ Qo server running at http://localhost:8080 (written in Go, for the Go game about Go)")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
