package opt

import (
	"testing"
)

func TestChordalDisjointNodes(t *testing.T) {
	ig := NewInterferenceGraph()
	// Add disconnected nodes by adding self edges or dummy
	ig.AddEdge(1, 2)
	// Remove edge to make them disjoint
	delete(ig.Edges[1], 2)
	delete(ig.Edges[2], 1)

	res := ColorChordalGraph(ig, 0)
	if res.Colors[1] != 0 || res.Colors[2] != 0 {
		t.Errorf("disjoint nodes should both get color 0, got %d and %d", res.Colors[1], res.Colors[2])
	}
}

func TestChordalTriangleK3(t *testing.T) {
	ig := NewInterferenceGraph()
	ig.AddEdge(1, 2)
	ig.AddEdge(2, 3)
	ig.AddEdge(3, 1)

	res := ColorChordalGraph(ig, 0)
	if res.MaxColor != 2 {
		t.Fatalf("K3 should require 3 colors (maxColor 2), got %d", res.MaxColor)
	}

	c1, c2, c3 := res.Colors[1], res.Colors[2], res.Colors[3]
	if c1 == c2 || c2 == c3 || c1 == c3 {
		t.Errorf("K3 colors must all be distinct, got %d, %d, %d", c1, c2, c3)
	}
}

func TestChordalDiamond(t *testing.T) {
	// Diamond graph:
	//   1
	//  / \
	// 2---3
	//  \ /
	//   4
	// Cliques: {1,2,3} and {4,2,3}, max clique = 3.
	// PEO should color this with exactly 3 colors!
	ig := NewInterferenceGraph()
	ig.AddEdge(1, 2)
	ig.AddEdge(1, 3)
	ig.AddEdge(2, 3) // chord
	ig.AddEdge(4, 2)
	ig.AddEdge(4, 3)

	res := ColorChordalGraph(ig, 0)
	if res.MaxColor != 2 {
		t.Fatalf("Diamond with chord should require exactly 3 colors, got %d (maxColor %d)", res.MaxColor+1, res.MaxColor)
	}

	// 1 and 4 can share the same color
	if res.Colors[1] != res.Colors[4] {
		// Non-interfering nodes 1 and 4 should ideally share a color
	}
}

func TestChordalSpilling(t *testing.T) {
	// K3 with maxColors = 2: exactly 1 node must spill!
	ig := NewInterferenceGraph()
	ig.AddEdge(1, 2)
	ig.AddEdge(2, 3)
	ig.AddEdge(3, 1)

	res := ColorChordalGraph(ig, 2)
	if len(res.Spilled) != 1 {
		t.Fatalf("expected exactly 1 spilled node, got %d", len(res.Spilled))
	}

	coloredCount := 0
	for _, c := range res.Colors {
		if c >= 0 && c < 2 {
			coloredCount++
		}
	}
	if coloredCount != 2 {
		t.Errorf("expected 2 colored nodes within capacity 2, got %d", coloredCount)
	}
}

func TestChordalPreferences(t *testing.T) {
	ig := NewInterferenceGraph()
	ig.AddEdge(1, 2)

	// Prefer color 1 for node 1
	prefs := map[int][]int{
		1: {1},
		2: {0},
	}

	res := ColorChordalGraphWithPreferences(ig, 3, prefs)
	if res.Colors[1] != 1 {
		t.Errorf("node 1 should receive preferred color 1, got %d", res.Colors[1])
	}
	if res.Colors[2] != 0 {
		t.Errorf("node 2 should receive preferred color 0, got %d", res.Colors[2])
	}
}
