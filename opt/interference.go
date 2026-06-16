package opt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

// InterferenceGraph represents a graph where edges denote that two variables (by ID) are live at the same time.
type InterferenceGraph struct {
	Edges map[int]map[int]bool
}

// NewInterferenceGraph initializes an empty interference graph.
func NewInterferenceGraph() *InterferenceGraph {
	return &InterferenceGraph{
		Edges: make(map[int]map[int]bool),
	}
}

// AddEdge adds an undirected edge between two variables in the interference graph.
func (ig *InterferenceGraph) AddEdge(u, v int) {
	if u == v {
		return
	}
	if ig.Edges[u] == nil {
		ig.Edges[u] = make(map[int]bool)
	}
	if ig.Edges[v] == nil {
		ig.Edges[v] = make(map[int]bool)
	}
	ig.Edges[u][v] = true
	ig.Edges[v][u] = true
}

// Format returns a string representation of the graph suitable for debugging.
func (ig *InterferenceGraph) Format() string {
	var nodes []int
	for n := range ig.Edges {
		nodes = append(nodes, n)
	}
	sort.Ints(nodes)

	var sb strings.Builder
	for _, n := range nodes {
		var neighbors []int
		for neighbor := range ig.Edges[n] {
			neighbors = append(neighbors, neighbor)
		}
		sort.Ints(neighbors)

		var nStrs []string
		for _, neighbor := range neighbors {
			nStrs = append(nStrs, fmt.Sprintf("%d", neighbor))
		}

		sb.WriteString(fmt.Sprintf("  %d -> [%s]\n", n, strings.Join(nStrs, ", ")))
	}
	return sb.String()
}

// ComputeInterferenceGraph builds the interference graph for the given SSA function.
func ComputeInterferenceGraph(f *ir.Function) *InterferenceGraph {
	ig := NewInterferenceGraph()
	liveness := ComputeLiveness(f)

	for _, b := range f.Blocks {
		// LiveSet starts as LiveOut[B]
		liveSet := make(map[int]bool)
		for id := range liveness.LiveOut[b] {
			liveSet[id] = true
		}

		// Process terminator
		if b.Terminator != nil {
			for id1 := range liveSet {
				for id2 := range liveSet {
					ig.AddEdge(id1, id2)
				}
			}

			for _, op := range OperandsOf(b.Terminator) {
				if inst, ok := op.(ir.Instruction); ok {
					liveSet[inst.GetID()] = true
				} else if param, ok := op.(*ir.Parameter); ok {
					liveSet[param.ID] = true
				}
			}
		}

		// Walk backwards
		for i := len(b.Instructions) - 1; i >= 0; i-- {
			instr := b.Instructions[i]

			// If it's a Phi node, the definition doesn't interfere with its operands
			// (operands are live on incoming edges, not inside this block).
			// We just add interferences for the defined value with current liveSet.
			defID := instr.GetID()

			for id := range liveSet {
				ig.AddEdge(defID, id)
			}

			// Remove defined variable from liveSet
			delete(liveSet, defID)

			// Add uses to liveSet (unless it's a Phi node)
			if _, isPhi := instr.(*ir.Phi); !isPhi {
				for _, op := range OperandsOf(instr) {
					if inst, ok := op.(ir.Instruction); ok {
						liveSet[inst.GetID()] = true
					} else if param, ok := op.(*ir.Parameter); ok {
						liveSet[param.ID] = true
					}
				}
			}
		}
	}

	// At the entry block, parameters interfere with whatever is live initially
	if len(f.Blocks) > 0 {
		entryLive := make(map[int]bool)
		for id := range liveness.LiveIn[f.Blocks[0]] {
			entryLive[id] = true
		}

		for _, param := range f.Parameters {
			for id := range entryLive {
				ig.AddEdge(param.ID, id)
			}
			// Parameters also interfere with each other
			entryLive[param.ID] = true
		}
	}

	return ig
}
