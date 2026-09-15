package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// ParallelMove represents a copy from Src to Dest that conceptually occurs
// simultaneously with other parallel moves at a block boundary.
type ParallelMove struct {
	Dest any // Destination identifier (ir.Value, slot string/int, register name)
	Src  any // Source identifier (ir.Value, slot string/int, register name, constant)
	Size int // Size of value in bytes (1, 2, 4, 8)
}

// StepKind identifies the operation needed to execute a step in the sequentialized schedule.
type StepKind int

const (
	StepCopy    StepKind = iota // Copy Src to Dest (dst <- src)
	StepSwap                    // Swap Loc1 and Loc2 (loc1 <-> loc2, e.g. EXG on M6809)
	StepSave                    // Save Src to temporary storage (temp <- src, e.g. PSHS)
	StepRestore                 // Restore from temporary storage to Dest (dest <- temp, e.g. PULS)
)

// SequentialStep is a single atomic action in the sequentialized execution order.
type SequentialStep struct {
	Kind StepKind
	Dest any
	Src  any
	Loc1 any
	Loc2 any
	Size int
}

// locsEqual checks if two locations represent the exact same storage or value.
func locsEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a == b {
		return true
	}
	if i1, ok := a.(ir.Instruction); ok {
		if i2, ok := b.(ir.Instruction); ok {
			return i1.GetID() == i2.GetID()
		}
	}
	return false
}

// SequentializeParallelCopies decomposes a set of simultaneous parallel moves
// into an equivalent sequence of sequential copies, swaps, and temporary saves/restores,
// guaranteeing that no value is overwritten before it is read (the Lost Copy problem)
// and that cyclic dependencies are resolved without data loss.
func SequentializeParallelCopies(moves []ParallelMove) []SequentialStep {
	// Step 1: Filter out identity moves (Dest == Src)
	var pending []ParallelMove
	for _, m := range moves {
		if !locsEqual(m.Dest, m.Src) {
			pending = append(pending, m)
		}
	}

	var steps []SequentialStep

	// Step 2: Loop until all pending moves are scheduled
	for len(pending) > 0 {
		// Find a move whose destination is NOT read by any other pending move.
		// Such a move is ready to be emitted immediately because overwriting Dest
		// will not destroy any source value still needed.
		foundReady := false
		for i, m := range pending {
			isRead := false
			for j, other := range pending {
				if i != j && locsEqual(m.Dest, other.Src) {
					isRead = true
					break
				}
			}

			if !isRead {
				steps = append(steps, SequentialStep{
					Kind: StepCopy,
					Dest: m.Dest,
					Src:  m.Src,
					Size: m.Size,
				})
				// Remove m from pending
				pending = append(pending[:i], pending[i+1:]...)
				foundReady = true
				break
			}
		}

		if foundReady {
			continue
		}

		// Step 3: If no ready move exists, all remaining moves form one or more directed cycles!
		// We trace a cycle starting from pending[0].
		visited := make([]int, 0, len(pending))
		visitedMap := make(map[int]int) // move index -> position in visited

		curr := 0
		for {
			if pos, seen := visitedMap[curr]; seen {
				// Cycle detected from visited[pos] to visited[len(visited)-1]!
				cycleIndices := visited[pos:]
				cycleLen := len(cycleIndices)

				if cycleLen == 2 {
					// 2-Cycle: mutual swap between two locations!
					m0 := pending[cycleIndices[0]]
					m1 := pending[cycleIndices[1]]

					steps = append(steps, SequentialStep{
						Kind: StepSwap,
						Loc1: m0.Dest,
						Loc2: m1.Dest,
						Size: m0.Size,
					})

					// Remove both moves from pending (larger index first to preserve index validity)
					idx0, idx1 := cycleIndices[0], cycleIndices[1]
					if idx0 < idx1 {
						idx0, idx1 = idx1, idx0
					}
					pending = append(pending[:idx0], pending[idx0+1:]...)
					pending = append(pending[:idx1], pending[idx1+1:]...)
				} else {
					// N-Cycle (N >= 3): Break cycle by saving the first destination to temporary storage.
					// Moves in cycle: c_0 <- c_1 <- c_2 <- ... <- c_{k-1} <- c_0
					// where cycleIndices[j] has Dest = c_j and Src = c_{j+1}
					firstMove := pending[cycleIndices[0]]

					// 1. Save c_0 (temp <- c_0)
					steps = append(steps, SequentialStep{
						Kind: StepSave,
						Src:  firstMove.Dest,
						Size: firstMove.Size,
					})

					// 2. Sequential copies for the rest of the cycle
					for j := 0; j < cycleLen-1; j++ {
						m := pending[cycleIndices[j]]
						steps = append(steps, SequentialStep{
							Kind: StepCopy,
							Dest: m.Dest,
							Src:  m.Src,
							Size: m.Size,
						})
					}

					// 3. Restore to the last destination from temporary storage (c_{k-1} <- temp)
					lastMove := pending[cycleIndices[cycleLen-1]]
					steps = append(steps, SequentialStep{
						Kind: StepRestore,
						Dest: lastMove.Dest,
						Size: lastMove.Size,
					})

					// Remove all cycle moves from pending
					toRemove := make(map[int]bool)
					for _, idx := range cycleIndices {
						toRemove[idx] = true
					}
					var newPending []ParallelMove
					for idx, m := range pending {
						if !toRemove[idx] {
							newPending = append(newPending, m)
						}
					}
					pending = newPending
				}
				break
			}

			visitedMap[curr] = len(visited)
			visited = append(visited, curr)

			// Find next move in cycle: the move whose Dest matches curr's Src
			currDestToFind := pending[curr].Src
			nextIdx := -1
			for j, cand := range pending {
				if locsEqual(cand.Dest, currDestToFind) {
					nextIdx = j
					break
				}
			}

			if nextIdx == -1 {
				// Should not happen in a true cycle, fallback
				break
			}
			curr = nextIdx
		}
	}

	return steps
}
