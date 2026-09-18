package opt

import (
	"sort"

	"github.com/strickyak/minigolf/ir"
)

// MaximumCardinalitySearch computes a Simplicial Elimination Ordering (PEO)
// for the given interference graph in O(V + E) time.
// Ties are broken deterministically by node ID ascending.
func MaximumCardinalitySearch(ig *InterferenceGraph) []int {
	// Collect all unique nodes
	var nodes []int
	for n := range ig.Edges {
		nodes = append(nodes, n)
	}
	sort.Ints(nodes)

	numNodes := len(nodes)
	if numNodes == 0 {
		return nil
	}

	// weight[v] tracks the number of already numbered neighbors of v
	weight := make(map[int]int, numNodes)
	for _, n := range nodes {
		weight[n] = 0
	}

	numbered := make(map[int]bool, numNodes)
	peo := make([]int, numNodes)

	// Number from numNodes-1 down to 0
	for i := numNodes - 1; i >= 0; i-- {
		// Find unnumbered vertex with maximum weight
		bestNode := -1
		bestWeight := -1

		for _, n := range nodes {
			if !numbered[n] {
				w := weight[n]
				if w > bestWeight || (w == bestWeight && (bestNode == -1 || n < bestNode)) {
					bestWeight = w
					bestNode = n
				}
			}
		}

		peo[i] = bestNode
		numbered[bestNode] = true

		// Increment weights of unnumbered neighbors
		for neighbor := range ig.Edges[bestNode] {
			if !numbered[neighbor] {
				weight[neighbor]++
			}
		}
	}

	return peo
}

// ChordalColoringResult contains the register assignment for each SSA value.
type ChordalColoringResult struct {
	Colors   map[int]int  // Node ID -> assigned color (0, 1, ..., K-1)
	Spilled  map[int]bool // Node ID -> true if spilled to stack
	MaxColor int          // Maximum color index used (chromatic number - 1)
}

// ColorChordalGraph greedily colors the interference graph along the
// Perfect Elimination Ordering produced by Maximum Cardinality Search.
// If maxColors > 0, nodes that cannot be colored within maxColors are marked as spilled.
func ColorChordalGraph(ig *InterferenceGraph, maxColors int) *ChordalColoringResult {
	peo := MaximumCardinalitySearch(ig)
	result := &ChordalColoringResult{
		Colors:   make(map[int]int),
		Spilled:  make(map[int]bool),
		MaxColor: -1,
	}

	if len(peo) == 0 {
		return result
	}

	// Traverse PEO in order
	for _, node := range peo {
		// Find colors used by neighbors
		usedColors := make(map[int]bool)
		for neighbor := range ig.Edges[node] {
			if c, ok := result.Colors[neighbor]; ok && c >= 0 {
				usedColors[c] = true
			}
		}

		// Find lowest available color
		assigned := -1
		for c := 0; ; c++ {
			if !usedColors[c] {
				if maxColors > 0 && c >= maxColors {
					// Cannot color within capacity: must spill
					break
				}
				assigned = c
				break
			}
		}

		if assigned >= 0 {
			result.Colors[node] = assigned
			if assigned > result.MaxColor {
				result.MaxColor = assigned
			}
		} else {
			result.Colors[node] = -1
			result.Spilled[node] = true
		}
	}

	return result
}

// ColorChordalGraphWithPreferences colors the interference graph along the PEO,
// attempting to assign each node a color from its preferred list before falling
// back to other available colors.
func ColorChordalGraphWithPreferences(ig *InterferenceGraph, maxColors int, preferences map[int][]int) *ChordalColoringResult {
	peo := MaximumCardinalitySearch(ig)
	result := &ChordalColoringResult{
		Colors:   make(map[int]int),
		Spilled:  make(map[int]bool),
		MaxColor: -1,
	}

	if len(peo) == 0 {
		return result
	}

	for _, node := range peo {
		usedColors := make(map[int]bool)
		for neighbor := range ig.Edges[node] {
			if c, ok := result.Colors[neighbor]; ok && c >= 0 {
				usedColors[c] = true
			}
		}

		assigned := -1

		// First, check if any preferred color is free
		if prefs, ok := preferences[node]; ok {
			for _, pref := range prefs {
				if pref < maxColors && !usedColors[pref] {
					assigned = pref
					break
				}
			}
		}

		// If no preferred color is available, pick lowest free color
		if assigned == -1 {
			for c := 0; ; c++ {
				if !usedColors[c] {
					if maxColors > 0 && c >= maxColors {
						break
					}
					assigned = c
					break
				}
			}
		}

		if assigned >= 0 {
			result.Colors[node] = assigned
			if assigned > result.MaxColor {
				result.MaxColor = assigned
			}
		} else {
			result.Colors[node] = -1
			result.Spilled[node] = true
		}
	}

	return result
}

// ComputeNextUseDistances calculates the distance to the next use for every variable
// at each instruction point in the function, used by Belady's furthest-next-use spilling.
func ComputeNextUseDistances(f *ir.Function) map[int]int {
	// Map from variable ID to position of its earliest upcoming use
	nextUse := make(map[int]int)
	pos := 0

	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			pos++
			for _, op := range OperandsOf(instr) {
				if inst, ok := op.(ir.Instruction); ok {
					id := inst.GetID()
					if _, seen := nextUse[id]; !seen {
						nextUse[id] = pos
					}
				} else if param, ok := op.(*ir.Parameter); ok {
					if _, seen := nextUse[param.ID]; !seen {
						nextUse[param.ID] = pos
					}
				}
			}
		}
		if b.Terminator != nil {
			pos++
			for _, op := range OperandsOf(b.Terminator) {
				if inst, ok := op.(ir.Instruction); ok {
					id := inst.GetID()
					if _, seen := nextUse[id]; !seen {
						nextUse[id] = pos
					}
				} else if param, ok := op.(*ir.Parameter); ok {
					if _, seen := nextUse[param.ID]; !seen {
						nextUse[param.ID] = pos
					}
				}
			}
		}
	}

	return nextUse
}
