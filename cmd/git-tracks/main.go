package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// ANSI escape codes for colors and styles.
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
	ansiWhite   = "\033[37m"

	ansiBgBlue    = "\033[44;97;1m"
	ansiBgGreen   = "\033[42;30;1m"
	ansiBgYellow  = "\033[43;30;1m"
	ansiBgMagenta = "\033[45;97;1m"
	ansiBgCyan    = "\033[46;30;1m"
	ansiBgGray    = "\033[100;97m"
)

var trackColors = []string{
	"\033[96;1m", // Bright Cyan
	"\033[93;1m", // Bright Yellow
	"\033[92;1m", // Bright Green
	"\033[95;1m", // Bright Magenta
	"\033[94;1m", // Bright Blue
	"\033[91;1m", // Bright Red
}

type BranchInfo struct {
	Name   string
	Hash   string
	IsHead bool
}

type CommitNode struct {
	Hash     string
	Parents  []string
	Children []string
	Branches []string
	IsHead   bool
	TopoIdx  int
}

type Edge struct {
	From  string
	To    string
	Count int // Number of commit steps along this collapsed segment
}

type ContractedGraph struct {
	Nodes    []string          // Topologically ordered station hashes (oldest -> newest)
	OutEdges map[string][]Edge // Station hash -> outgoing edges
	InEdges  map[string][]Edge // Station hash -> incoming edges
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func loadRepo(includeRemotes bool) ([]BranchInfo, map[string]*CommitNode, []string, error) {
	headRef, _ := runGit("symbolic-ref", "--short", "HEAD")

	refArgs := []string{"for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/"}
	if includeRemotes {
		refArgs = append(refArgs, "refs/remotes/")
	}
	refOut, err := runGit(refArgs...)
	if err != nil {
		return nil, nil, nil, err
	}

	var branches []BranchInfo
	branchByCommit := make(map[string][]string)
	headCommits := make(map[string]bool)

	for _, line := range strings.Split(refOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name, hash := parts[0], parts[1]
		if strings.HasSuffix(name, "/HEAD") || name == "origin" {
			continue
		}
		isHead := (name == headRef)
		branches = append(branches, BranchInfo{Name: name, Hash: hash, IsHead: isHead})
		branchByCommit[hash] = append(branchByCommit[hash], name)
		if isHead {
			headCommits[hash] = true
		}
	}

	sort.Slice(branches, func(i, j int) bool {
		return branches[i].Name < branches[j].Name
	})

	revArgs := []string{"rev-list", "--parents", "--topo-order", "--reverse", "--branches"}
	if includeRemotes {
		revArgs = []string{"rev-list", "--parents", "--topo-order", "--reverse", "--all"}
	}
	revOut, err := runGit(revArgs...)
	if err != nil {
		return nil, nil, nil, err
	}

	commits := make(map[string]*CommitNode)
	var order []string
	for idx, line := range strings.Split(revOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		hash := parts[0]
		parents := parts[1:]
		node := &CommitNode{
			Hash:     hash,
			Parents:  parents,
			Branches: branchByCommit[hash],
			IsHead:   headCommits[hash],
			TopoIdx:  idx,
		}
		commits[hash] = node
		order = append(order, hash)
	}

	for _, h := range order {
		node := commits[h]
		for _, p := range node.Parents {
			if pn, ok := commits[p]; ok {
				pn.Children = append(pn.Children, h)
			}
		}
	}

	return branches, commits, order, nil
}

// buildFullContractedGraph keeps roots, branch tips/named commits, forks (children > 1), and merges (parents > 1).
func buildFullContractedGraph(commits map[string]*CommitNode, order []string) *ContractedGraph {
	isStation := make(map[string]bool)
	for _, h := range order {
		c := commits[h]
		if len(c.Parents) != 1 || len(c.Children) != 1 || len(c.Branches) > 0 {
			isStation[h] = true
		}
	}

	cg := &ContractedGraph{
		OutEdges: make(map[string][]Edge),
		InEdges:  make(map[string][]Edge),
	}

	for _, h := range order {
		if !isStation[h] {
			continue
		}
		cg.Nodes = append(cg.Nodes, h)
		c := commits[h]
		for _, p := range c.Parents {
			steps := 1
			cur := p
			for !isStation[cur] {
				steps++
				cur = commits[cur].Parents[0]
			}
			edge := Edge{From: cur, To: h, Count: steps}
			cg.OutEdges[cur] = append(cg.OutEdges[cur], edge)
			cg.InEdges[h] = append(cg.InEdges[h], edge)
		}
	}

	return cg
}

// buildBranchSkeletonGraph collapses internal self-merges whose commits all belong to the exact same set of branches.
// Every commit is labeled by the bitmask of branches that can reach it (its descendant branches).
func buildBranchSkeletonGraph(branches []BranchInfo, commits map[string]*CommitNode, order []string) *ContractedGraph {
	// Compute reachability bitmask: which branch tips can reach commit h?
	reachMask := make(map[string]uint64)
	for i, b := range branches {
		if i < 64 {
			reachMask[b.Hash] |= (1 << uint(i))
		}
	}
	// Propagate from newest to oldest
	for i := len(order) - 1; i >= 0; i-- {
		h := order[i]
		m := reachMask[h]
		for _, p := range commits[h].Parents {
			reachMask[p] |= m
		}
	}

	// Group commits by identical non-zero reachMask, and find transitions between masks.
	maskCount := make(map[uint64]int)
	maskRepNewest := make(map[uint64]string) // newest commit with this mask
	maskRepOldest := make(map[uint64]string) // oldest commit with this mask

	for _, h := range order {
		m := reachMask[h]
		maskCount[m]++
		if _, ok := maskRepOldest[m]; !ok {
			maskRepOldest[m] = h
		}
		maskRepNewest[m] = h
	}

	// Each distinct reachMask has a newest commit (the station where Either a branch sits OR branches diverge!).
	// Wait: let's also include the oldest root commit as the starting station!
	rootHash := order[0]
	isStation := make(map[string]bool)
	isStation[rootHash] = true
	for _, h := range order {
		if len(commits[h].Branches) > 0 {
			isStation[h] = true
		}
	}
	for _, newestH := range maskRepNewest {
		isStation[newestH] = true
	}

	// Build DAG between these stations based on reachMask transitions
	var stations []string
	for _, h := range order {
		if isStation[h] {
			stations = append(stations, h)
		}
	}

	// For each station, find its immediate predecessor station(s) via parent paths,
	// and count the exact number of commits in the equivalence class between them.
	// Using BFS/DFS backwards from station `h` stopping at any other station.
	cg := &ContractedGraph{
		Nodes:    stations,
		OutEdges: make(map[string][]Edge),
		InEdges:  make(map[string][]Edge),
	}

	for _, h := range stations {
		if h == rootHash {
			continue
		}
		// Find all commits strictly after ancestor stations up to h
		visited := make(map[string]bool)
		predStations := make(map[string]bool)
		var stack []string
		visited[h] = true
		for _, p := range commits[h].Parents {
			stack = append(stack, p)
		}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if isStation[cur] {
				predStations[cur] = true
				continue
			}
			if visited[cur] {
				continue
			}
			visited[cur] = true
			for _, p := range commits[cur].Parents {
				stack = append(stack, p)
			}
		}

		// Keep only immediate predecessor stations (not ancestors of another predStation)
		var preds []string
		for ps := range predStations {
			preds = append(preds, ps)
		}
		sort.Slice(preds, func(i, j int) bool {
			return commits[preds[i]].TopoIdx > commits[preds[j]].TopoIdx
		})

		// Filter out transitive ancestors among preds
		var directPreds []string
		for _, cand := range preds {
			isTransitive := false
			for _, other := range directPreds {
				// check if cand is ancestor of other
				if isAncestorInMap(cand, other, commits) {
					isTransitive = true
					break
				}
			}
			if !isTransitive {
				directPreds = append(directPreds, cand)
			}
		}

		sort.Slice(directPreds, func(i, j int) bool {
			return commits[directPreds[i]].TopoIdx < commits[directPreds[j]].TopoIdx
		})

		cnt := len(visited)
		for _, p := range directPreds {
			e := Edge{From: p, To: h, Count: cnt}
			cg.OutEdges[p] = append(cg.OutEdges[p], e)
			cg.InEdges[h] = append(cg.InEdges[h], e)
		}
	}

	return cg
}

func isAncestorInMap(anc, desc string, commits map[string]*CommitNode) bool {
	ancIdx := commits[anc].TopoIdx
	visited := make(map[string]bool)
	stack := []string{desc}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == anc {
			return true
		}
		if visited[cur] || commits[cur].TopoIdx < ancIdx {
			continue
		}
		visited[cur] = true
		stack = append(stack, commits[cur].Parents...)
	}
	return false
}

// Styled cell in the 2D railroad grid
type Cell struct {
	Ch    rune
	Style string
}

type Canvas struct {
	Rows int
	Cols int
	Grid [][]Cell
}

func NewCanvas(rows, cols int) *Canvas {
	g := make([][]Cell, rows)
	for r := 0; r < rows; r++ {
		g[r] = make([]Cell, cols)
		for c := 0; c < cols; c++ {
			g[r][c] = Cell{Ch: ' '}
		}
	}
	return &Canvas{Rows: rows, Cols: cols, Grid: g}
}

func (cv *Canvas) EnsureCols(cols int) {
	if cols <= cv.Cols {
		return
	}
	for r := 0; r < cv.Rows; r++ {
		extra := make([]Cell, cols-cv.Cols)
		for i := range extra {
			extra[i] = Cell{Ch: ' '}
		}
		cv.Grid[r] = append(cv.Grid[r], extra...)
	}
	cv.Cols = cols
}

func (cv *Canvas) EnsureRows(rows int) {
	for cv.Rows < rows {
		row := make([]Cell, cv.Cols)
		for c := 0; c < cv.Cols; c++ {
			row[c] = Cell{Ch: ' '}
		}
		cv.Grid = append(cv.Grid, row)
		cv.Rows++
	}
}

func (cv *Canvas) Put(r, c int, ch rune, style string) {
	cv.EnsureRows(r + 1)
	cv.EnsureCols(c + 1)
	cv.Grid[r][c] = Cell{Ch: ch, Style: style}
}

func (cv *Canvas) PutString(r, c int, s string, style string) int {
	runes := []rune(s)
	cv.EnsureRows(r + 1)
	cv.EnsureCols(c + len(runes))
	for i, ch := range runes {
		cv.Grid[r][c+i] = Cell{Ch: ch, Style: style}
	}
	return len(runes)
}

type ActiveLane struct {
	To    string
	Count int
	Color string
	Drawn bool // whether (Count) has already been drawn on this lane
}

func formatStationLabel(h string, commits map[string]*CommitNode) (string, string) {
	c := commits[h]
	if len(c.Branches) > 0 {
		var parts []string
		for _, b := range c.Branches {
			if c.IsHead && b == c.Branches[len(c.Branches)-1] {
				parts = append(parts, b+"*")
			} else {
				parts = append(parts, b)
			}
		}
		lbl := "[" + strings.Join(parts, "=") + "]"
		if c.IsHead {
			return lbl, ansiBgGreen
		}
		if len(c.Branches) > 1 {
			return lbl, ansiBgMagenta
		}
		if c.Branches[0] == "main" {
			return lbl, ansiBgBlue
		}
		return lbl, ansiBgYellow
	}
	return "[" + h[:6] + "]", ansiBgGray
}

// RenderContractedGraph renders a ContractedGraph left-to-right onto a Canvas.
func RenderContractedGraph(cg *ContractedGraph, commits map[string]*CommitNode) *Canvas {
	cv := NewCanvas(2, 1)
	var lanes []*ActiveLane // each index is a track row (lane r uses canvas row r*2)
	var laneFreeCol []int   // minimum column at which a freed lane can be reused
	colorIdx := 0
	nextColor := func() string {
		c := trackColors[colorIdx%len(trackColors)]
		colorIdx++
		return c
	}

	allocLane := func(preferred int, atCol int) int {
		if preferred >= 0 && preferred < len(lanes) && lanes[preferred] == nil && atCol >= laneFreeCol[preferred] {
			return preferred
		}
		for i := range lanes {
			if lanes[i] == nil && atCol >= laneFreeCol[i] {
				return i
			}
		}
		lanes = append(lanes, nil)
		laneFreeCol = append(laneFreeCol, 0)
		cv.EnsureRows(len(lanes)*2 + 1)
		return len(lanes) - 1
	}

	drawBgTracks := func(colStart, width int, skipLane int) {
		for l, al := range lanes {
			if al == nil || l == skipLane {
				continue
			}
			row := l * 2
			for c := colStart; c < colStart+width; c++ {
				cv.Put(row, c, '═', al.Color)
			}
		}
	}

	// Helper to flush any undrawn `(count)` labels on currently active lanes before a merge/station.
	flushPendingCounts := func(curCol int, onlyForTarget string) int {
		maxLen := 0
		needDraw := false
		for _, al := range lanes {
			if al != nil && !al.Drawn && (onlyForTarget == "" || al.To == onlyForTarget) {
				needDraw = true
				s := fmt.Sprintf("═(%d)═", al.Count)
				if len(s) > maxLen {
					maxLen = len(s)
				}
			}
		}
		if !needDraw {
			return curCol
		}
		for l, al := range lanes {
			if al == nil {
				continue
			}
			row := l * 2
			if !al.Drawn && (onlyForTarget == "" || al.To == onlyForTarget) {
				lbl := fmt.Sprintf("(%d)", al.Count)
				padTotal := maxLen - len(lbl)
				padLeft := padTotal / 2
				padRight := padTotal - padLeft
				for i := 0; i < padLeft; i++ {
					cv.Put(row, curCol+i, '═', al.Color)
				}
				cv.PutString(row, curCol+padLeft, lbl, ansiBold+ansiWhite)
				for i := 0; i < padRight; i++ {
					cv.Put(row, curCol+padLeft+len(lbl)+i, '═', al.Color)
				}
				al.Drawn = true
			} else {
				for i := 0; i < maxLen; i++ {
					cv.Put(row, curCol+i, '═', al.Color)
				}
			}
		}
		return curCol + maxLen
	}

	curCol := 0

	for _, h := range cg.Nodes {
		// 1. Find all lanes arriving at station `h`
		var inLanes []int
		for l, al := range lanes {
			if al != nil && al.To == h {
				inLanes = append(inLanes, l)
			}
		}

		// Ensure any incoming edges to `h` have had their `(count)` drawn before joining!
		curCol = flushPendingCounts(curCol, h)

		var mainLane int
		var mainColor string

		if len(inLanes) == 0 {
			// Root station
			mainLane = allocLane(0, curCol)
			mainColor = nextColor()
		} else if len(inLanes) == 1 {
			mainLane = inLanes[0]
			mainColor = lanes[mainLane].Color
			lanes[mainLane] = nil
		} else {
			// Merge point! Multiple lanes join into inLanes[0].
			sort.Ints(inLanes)
			mainLane = inLanes[0]
			mainColor = lanes[mainLane].Color
			minL := inLanes[0]
			maxL := inLanes[len(inLanes)-1]

			// Draw 2 columns for the merge switch: column curCol has the vertical merge rail, curCol+1 has `═`
			drawBgTracks(curCol, 2, -1)
			mergeCol := curCol
			inSet := make(map[int]bool)
			for _, il := range inLanes {
				inSet[il] = true
			}

			for r := minL * 2; r <= maxL*2; r++ {
				if r%2 == 0 {
					l := r / 2
					if l == minL {
						cv.Put(r, mergeCol, '╦', mainColor)
						cv.Put(r, mergeCol+1, '═', mainColor)
					} else if l == maxL {
						cv.Put(r, mergeCol, '╝', lanes[l].Color)
						cv.Put(r, mergeCol+1, ' ', "")
					} else if inSet[l] {
						cv.Put(r, mergeCol, '╣', lanes[l].Color)
						cv.Put(r, mergeCol+1, ' ', "")
					} else if lanes[l] != nil {
						// Crossing over another active horizontal track!
						cv.Put(r, mergeCol, '╬', lanes[l].Color)
					} else {
						cv.Put(r, mergeCol, '║', mainColor)
					}
				} else {
					cv.Put(r, mergeCol, '║', mainColor)
				}
			}

			for _, il := range inLanes {
				lanes[il] = nil
			}
			curCol += 2
		}

		// 2. Draw the station badge at (mainLane*2, curCol)
		lbl, badgeStyle := formatStationLabel(h, commits)
		lblWidth := len([]rune(lbl))
		outs := cg.OutEdges[h]

		if len(outs) == 0 && len(inLanes) <= 1 {
			// Terminal leaf station on its own lane: draw badge without advancing curCol for other parallel lanes!
			cv.PutString(mainLane*2, curCol, lbl, badgeStyle)
			laneFreeCol[mainLane] = curCol + lblWidth + 2
			continue
		}

		drawBgTracks(curCol, lblWidth, mainLane)
		cv.PutString(mainLane*2, curCol, lbl, badgeStyle)
		curCol += lblWidth

		// 3. Handle outgoing edges from station `h`
		if len(outs) == 1 {
			lanes[mainLane] = &ActiveLane{
				To:    outs[0].To,
				Count: outs[0].Count,
				Color: mainColor,
				Drawn: false,
			}
			curCol = flushPendingCounts(curCol, "")
		} else if len(outs) > 1 {
			// Branching (fork) point!
			outLanes := make([]int, len(outs))
			outLanes[0] = mainLane
			lanes[mainLane] = &ActiveLane{
				To:    outs[0].To,
				Count: outs[0].Count,
				Color: mainColor,
				Drawn: false,
			}
			for i := 1; i < len(outs); i++ {
				nl := allocLane(mainLane+i, curCol)
				outLanes[i] = nl
				lanes[nl] = &ActiveLane{
					To:    outs[i].To,
					Count: outs[i].Count,
					Color: nextColor(),
					Drawn: false,
				}
			}

			// Draw fork switch column
			minL, maxL := outLanes[0], outLanes[0]
			outSet := make(map[int]bool)
			for _, ol := range outLanes {
				outSet[ol] = true
				if ol < minL {
					minL = ol
				}
				if ol > maxL {
					maxL = ol
				}
			}

			drawBgTracks(curCol, 2, -1)
			forkCol := curCol
			for r := minL * 2; r <= maxL*2; r++ {
				if r%2 == 0 {
					l := r / 2
					if l == minL {
						if l == mainLane {
							cv.Put(r, forkCol, '═', mainColor)
							cv.Put(r, forkCol+1, '╦', mainColor)
						} else {
							cv.Put(r, forkCol, ' ', "")
							cv.Put(r, forkCol+1, '╔', lanes[l].Color)
						}
					} else if l == maxL {
						if l == mainLane {
							cv.Put(r, forkCol, '═', mainColor)
							cv.Put(r, forkCol+1, '╩', mainColor)
						} else {
							cv.Put(r, forkCol, ' ', "")
							cv.Put(r, forkCol+1, '╚', lanes[l].Color)
						}
					} else if outSet[l] {
						if l == mainLane {
							cv.Put(r, forkCol, '═', mainColor)
							cv.Put(r, forkCol+1, '╬', mainColor)
						} else {
							cv.Put(r, forkCol, ' ', "")
							cv.Put(r, forkCol+1, '╠', lanes[l].Color)
						}
					} else if lanes[l] != nil {
						cv.Put(r, forkCol+1, '╬', lanes[l].Color)
					} else {
						cv.Put(r, forkCol+1, '║', mainColor)
					}
				} else {
					cv.Put(r, forkCol+1, '║', mainColor)
				}
			}
			curCol += 2
			curCol = flushPendingCounts(curCol, "")
		}
	}

	// Trim trailing empty rows
	for cv.Rows > 1 {
		empty := true
		for c := 0; c < cv.Cols; c++ {
			if cv.Grid[cv.Rows-1][c].Ch != ' ' {
				empty = false
				break
			}
		}
		if empty {
			cv.Rows--
		} else {
			break
		}
	}

	return cv
}

func (cv *Canvas) PrintWrapped(maxWidth int) {
	if maxWidth <= 0 || cv.Cols <= maxWidth {
		cv.printSlice(0, cv.Cols, false, false)
		return
	}

	start := 0
	for start < cv.Cols {
		end := start + maxWidth
		if end >= cv.Cols {
			cv.printSlice(start, cv.Cols, start > 0, false)
			break
		}
		// Find a clean column to split on (all cells in column c and c-1 must be ' ' or '═')
		splitCol := end
		for c := end; c > start+maxWidth/2; c-- {
			clean := true
			for r := 0; r < cv.Rows; r++ {
				ch := cv.Grid[r][c].Ch
				chPrev := cv.Grid[r][c-1].Ch
				if (ch != ' ' && ch != '═') || (chPrev != ' ' && chPrev != '═') {
					clean = false
					break
				}
			}
			if clean {
				splitCol = c
				break
			}
		}
		cv.printSlice(start, splitCol, start > 0, true)
		fmt.Println()
		start = splitCol
	}
}

func (cv *Canvas) printSlice(cStart, cEnd int, hasLeftCont, hasRightCont bool) {
	// Determine last non-empty row in this horizontal slice
	maxRow := 0
	for r := 0; r < cv.Rows; r++ {
		for c := cStart; c < cEnd; c++ {
			if cv.Grid[r][c].Ch != ' ' {
				maxRow = r
				break
			}
		}
	}

	for r := 0; r <= maxRow; r++ {
		var sb strings.Builder
		sb.WriteString("  ")
		if hasLeftCont {
			if cv.Grid[r][cStart].Ch == '═' {
				sb.WriteString(ansiDim + "»" + ansiReset)
			} else {
				sb.WriteRune(' ')
			}
		}
		curStyle := ""
		for c := cStart; c < cEnd; c++ {
			cell := cv.Grid[r][c]
			if cell.Style != curStyle {
				sb.WriteString(ansiReset)
				if cell.Style != "" {
					sb.WriteString(cell.Style)
				}
				curStyle = cell.Style
			}
			sb.WriteRune(cell.Ch)
		}
		if curStyle != "" {
			sb.WriteString(ansiReset)
		}
		if hasRightCont {
			if cEnd > 0 && cv.Grid[r][cEnd-1].Ch == '═' {
				sb.WriteString(ansiDim + "»" + ansiReset)
			}
		}
		fmt.Println(sb.String())
	}
}

func printBranchSummary(branches []BranchInfo, commits map[string]*CommitNode) {
	fmt.Printf("%s=== Git Branch Inclusion & Topology (%d branches) ===%s\n", ansiBold+ansiCyan, len(branches), ansiReset)
	for _, b := range branches {
		var includes []string
		var equals []string
		for _, other := range branches {
			if other.Name == b.Name {
				continue
			}
			if other.Hash == b.Hash {
				equals = append(equals, other.Name)
			} else if isAncestorInMap(other.Hash, b.Hash, commits) {
				includes = append(includes, other.Name)
			}
		}
		headMark := " "
		if b.IsHead {
			headMark = "*"
		}
		var notes []string
		if len(equals) > 0 {
			notes = append(notes, fmt.Sprintf("same commit as %s", strings.Join(equals, ", ")))
		}
		if len(includes) > 0 {
			notes = append(notes, fmt.Sprintf("includes: %s", strings.Join(includes, ", ")))
		}
		noteStr := ""
		if len(notes) > 0 {
			noteStr = "  (" + strings.Join(notes, "; ") + ")"
		}
		fmt.Printf(" %s %s%-18s%s %s%s%s%s\n",
			headMark,
			ansiBold+ansiYellow, b.Name, ansiReset,
			ansiDim, b.Hash[:6], ansiReset,
			noteStr)
	}
	fmt.Println()
}

func main() {
	allFlag := flag.Bool("all", false, "Include remote tracking branches")
	fullFlag := flag.Bool("full", true, "Show full commit DAG with all historical fork/merge diamonds")
	widthFlag := flag.Int("w", 100, "Max panel width before wrapping (0 for no wrap)")
	flag.Parse()

	branches, commits, order, err := loadRepo(*allFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-tracks: %v\n", err)
		os.Exit(1)
	}
	if len(order) == 0 {
		fmt.Fprintf(os.Stderr, "git-tracks: no commits found\n")
		os.Exit(1)
	}

	printBranchSummary(branches, commits)

	// 1. High-level Branch Skeleton Railroad Diagram
	fmt.Printf("%s=== Branch Railroad Diagram (Time: Left ──► Right) ===%s\n", ansiBold+ansiCyan, ansiReset)
	skelGraph := buildBranchSkeletonGraph(branches, commits, order)
	skelCanvas := RenderContractedGraph(skelGraph, commits)
	skelCanvas.PrintWrapped(*widthFlag)

	// 2. Full Commit DAG Railroad Diagram (if it has more stations than the branch skeleton)
	if *fullFlag {
		fullGraph := buildFullContractedGraph(commits, order)
		if len(fullGraph.Nodes) > len(skelGraph.Nodes) {
			fmt.Println()
			fmt.Printf("%s=== Full Commit-DAG Railroad Diagram (All %d Fork/Merge/Named Stations) ===%s\n",
				ansiBold+ansiCyan, len(fullGraph.Nodes), ansiReset)
			fullCanvas := RenderContractedGraph(fullGraph, commits)
			fullCanvas.PrintWrapped(*widthFlag)
		}
	}
}
