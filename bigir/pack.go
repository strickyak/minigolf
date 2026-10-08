package bigir

import (
	"fmt"
)

const (
	BlockSize8KB      = 8192
	FarCodeBaseSlot5  = 0xA000
	InitialFarBlockID = 8
	MaxFarBlockID     = 127
)

// PackProgram assigns functions to physical 8KB blocks (8..127) and generates
// the Slot 6 trampoline table.
func PackProgram(prog *Program) error {
	prog.FarBlocks = make(map[int][]*Function)
	prog.Trampolines = make([]*Trampoline, 0)

	currentBlockID := InitialFarBlockID
	currentBlockOffset := uint16(0)

	for _, fn := range prog.Functions {
		if !fn.IsFar {
			// Near functions live in fixed memory (Slot 6, Block 6)
			fn.BlockID = 6
			continue
		}

		// Estimate code size (at least 64 bytes for frame + 28 bytes per instruction)
		estimatedSize := 64
		for _, bb := range fn.Blocks {
			estimatedSize += len(bb.Instructions) * 28
		}
		fn.AllocSize = estimatedSize

		// Check if function fits in current 8KB block (keep safely below 8192 bytes, e.g. 6000)
		if currentBlockOffset > 0 && int(currentBlockOffset)+estimatedSize > 6000 {
			currentBlockID++
			if currentBlockID > MaxFarBlockID {
				return fmt.Errorf("EMBIGGEN code pool overflow: exceeded maximum Far Code block %d (960 KB)", MaxFarBlockID)
			}
			currentBlockOffset = 0
		}

		fn.BlockID = currentBlockID
		fn.Slot5Offset = currentBlockOffset
		currentBlockOffset += uint16(estimatedSize)

		// Group into FarBlocks
		prog.FarBlocks[fn.BlockID] = append(prog.FarBlocks[fn.BlockID], fn)

		// Generate Slot 6 Trampoline Entry
		slot5Entry := uint16(FarCodeBaseSlot5 + fn.Slot5Offset)
		prog.Trampolines = append(prog.Trampolines, &Trampoline{
			FuncName:    fn.Name,
			TargetBlock: fn.BlockID,
			TargetAddr:  slot5Entry,
		})
	}

	// Intra-Block Call Devirtualization:
	// If caller and callee reside in the same physical 8KB block, devirtualize
	// FarCall into a direct near call to callee's Slot 5 address!
	funcMap := make(map[string]*Function)
	for _, fn := range prog.Functions {
		funcMap[fn.Name] = fn
	}

	for _, fn := range prog.Functions {
		for _, bb := range fn.Blocks {
			for idx, instr := range bb.Instructions {
				if fc, ok := instr.(*FarCall); ok {
					if callee, exists := funcMap[fc.Callee]; exists {
						if fn.IsFar && callee.IsFar && fn.BlockID == callee.BlockID {
							// Same block! Replace with NearCall directly to callee in Slot 5
							nearCall := &NearCall{
								BaseInstruction: fc.BaseInstruction,
								Callee:          callee.Name,
								Args:            fc.Args,
							}
							nearCall.SetComment("devirtualized intra-block call")
							bb.Instructions[idx] = nearCall
						}
					}
				}
			}
		}
	}

	return nil
}
