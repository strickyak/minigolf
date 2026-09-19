package opt

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

var _ = fmt.Sprintf

type InlineOptions struct {
	EnableTiny          bool
	EnableSingleCall    bool
	MaxTinyInstructions int // Maximum instructions for a tiny function (default: 8)
	MaxInlineRounds     int // Maximum inlining passes (default: 10)
	WordSize            int
}

func DefaultInlineOptions() InlineOptions {
	noInline := os.Getenv("NO_INLINE") != ""
	noTiny := os.Getenv("NO_INLINE_TINY") != "" || noInline
	noSingle := os.Getenv("NO_INLINE_SINGLE_CALL") != "" || noInline
	maxTiny := 8
	if v := os.Getenv("INLINE_MAX_TINY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxTiny = n
		}
	}
	maxRounds := 10
	if v := os.Getenv("INLINE_MAX_ROUNDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxRounds = n
		}
	}
	return InlineOptions{
		EnableTiny:          !noTiny,
		EnableSingleCall:    !noSingle,
		MaxTinyInstructions: maxTiny,
		MaxInlineRounds:     maxRounds,
	}
}

func cleanupFunction(f *ir.Function, wordSize int) {
	cf := &ConstFoldPass{WordSize: wordSize}
	cp := &CopyPropPass{}
	dce := &DCEPass{}
	dbe := &DBEPass{}
	ps := &PhiSimpPass{}
	bf := &BranchFoldPass{}
	for i := 0; i < 3; i++ {
		ch := false
		if cf.Run(f) {
			ch = true
		}
		if dbe.Run(f) {
			ch = true
		}
		if cp.Run(f) {
			ch = true
		}
		if ps.Run(f) {
			ch = true
		}
		if dce.Run(f) {
			ch = true
		}
		if bf.Run(f) {
			ch = true
		}
		if !ch {
			break
		}
	}
}

// InlinePass runs function inlining across the entire whole-program IR.
func InlinePass(p *ir.Program, opts InlineOptions) bool {
	if !opts.EnableTiny && !opts.EnableSingleCall {
		return false
	}

	overallChanged := false

	// Pre-inlining cleanup: fold constants and simplify before inspecting instruction counts
	for _, f := range p.Functions {
		cleanupFunction(f, opts.WordSize)
	}

	// Find the current maximum instruction ID and block ID
	maxID := 0
	maxBlockID := 0
	for _, f := range p.Functions {
		for _, b := range f.Blocks {
			if b.ID > maxBlockID {
				maxBlockID = b.ID
			}
			for _, instr := range b.Instructions {
				if instr.GetID() > maxID {
					maxID = instr.GetID()
				}
			}
			if b.Terminator != nil && b.Terminator.GetID() > maxID {
				maxID = b.Terminator.GetID()
			}
		}
	}
	nextID := func() int {
		maxID++
		return maxID
	}
	nextBlockID := func() int {
		maxBlockID++
		return maxBlockID
	}

	maxRounds := opts.MaxInlineRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}
	maxTiny := opts.MaxTinyInstructions
	if maxTiny <= 0 {
		maxTiny = 8
	}

	for round := 0; round < maxRounds; round++ {
		roundChanged := false

		// Analyze call counts across all functions in the program
		callCounts := make(map[*ir.Function]int)
		addrTaken := make(map[*ir.Function]bool)
		for _, f := range p.Functions {
			for _, b := range f.Blocks {
				for _, instr := range b.Instructions {
					switch i := instr.(type) {
					case *ir.Call:
						if i.Func != nil {
							callCounts[i.Func]++
						}
					case *ir.AddressOfFunc:
						if i.Func != nil {
							addrTaken[i.Func] = true
						}
					}
				}
			}
		}

		for _, caller := range p.Functions {
			inlinesThisCaller := 0
			for bIdx := 0; bIdx < len(caller.Blocks); bIdx++ {
				b := caller.Blocks[bIdx]
				for j := 0; j < len(b.Instructions); j++ {
					call, isCall := b.Instructions[j].(*ir.Call)
					if !isCall || call.Func == nil {
						continue
					}
					callee := call.Func
					if callee == caller {
						continue // Never self-inline
					}
					if !canInline(callee) {
						continue
					}

					callCount := callCounts[callee]
					isAddrTaken := addrTaken[callee]

					baseTiny := maxTiny
					effectiveMaxTiny := baseTiny
					effectiveMaxBlocks := 4

					maxPop := caller.Popularity
					if callee.Popularity > maxPop {
						maxPop = callee.Popularity
					}
					if maxPop >= 100 {
						effectiveMaxTiny = baseTiny * 3
						effectiveMaxBlocks = 8
					} else if maxPop >= 10 {
						effectiveMaxTiny = baseTiny * 2
						effectiveMaxBlocks = 6
					}

					if caller.LeafLevel == 2 && callee.LeafLevel == 1 {
						effectiveMaxTiny += 4
					}

					isSingle := opts.EnableSingleCall && callCount == 1 && !isAddrTaken
					singleBudgetInst := 64
					singleBudgetBlocks := 16
					if caller.TrunkLevel > 0 {
						singleBudgetInst = 128
						singleBudgetBlocks = 24
					}

					calleeIsStraight := isStraightLine(callee)
					calleeIsAcyclic := !calleeIsStraight && isAcyclic(callee)

					canInlineStraight := calleeIsStraight && ((opts.EnableTiny && isTinyFunction(callee, effectiveMaxTiny)) ||
						isSingle)

					canInlineMultiBlock := calleeIsAcyclic && ((opts.EnableTiny && canInlineCFG(callee, effectiveMaxTiny, effectiveMaxBlocks, false /* no calls */)) ||
						(isSingle && canInlineCFG(callee, singleBudgetInst, singleBudgetBlocks, true /* allow calls */)))

					if canInlineStraight {
						inlineStraightLine(caller, b, j, call, callee, nextID)
						cleanupFunction(caller, opts.WordSize)
						callCounts[callee]--
						inlinesThisCaller++
						roundChanged = true
						overallChanged = true
						if inlinesThisCaller >= 10 {
							break
						}
						bIdx = -1
						break
					} else if canInlineMultiBlock {
						inlineCFG(caller, b, j, call, callee, nextID, nextBlockID)
						cleanupFunction(caller, opts.WordSize)
						callCounts[callee]--
						inlinesThisCaller++
						roundChanged = true
						overallChanged = true
						if inlinesThisCaller >= 10 {
							break
						}
						bIdx = -1
						break
					}
				}
				if inlinesThisCaller >= 10 {
					break
				}
			}
		}

		if !roundChanged {
			break
		}
	}

	return overallChanged
}

// isInitFunction returns true if the function is an initialization function
// (such as init__main, prelude.init_0, or any module-level init_N function).
func isInitFunction(name string) bool {
	if name == "init" || name == "_init" || name == "init__main" || strings.HasPrefix(name, "init__") {
		return true
	}
	if strings.HasPrefix(name, "init_") || strings.Contains(name, ".init_") {
		return true
	}
	return false
}

// canInline checks whether a callee is eligible for inlining under any strategy.
func canInline(f *ir.Function) bool {
	if f == nil || len(f.Blocks) == 0 {
		return false
	}
	// Never inline entry points, runtime root functions, or initialization functions
	if f.Name == "main.main" || f.Name == "_main" || isInitFunction(f.Name) {
		return false
	}
	// Never inline functions with defers, destructors, setjmp, longjmp, or local variable addresses
	if hasDefersOrDestructors(f) {
		return false
	}
	return true
}

// hasDefersOrDestructors checks if the function has deferred actions, destructors, or local address operations.
func hasDefersOrDestructors(f *ir.Function) bool {
	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			switch i := instr.(type) {
			case *ir.SetJmp, *ir.LongJmp:
				return true
			case *ir.AddressOfLocal:
				return true
			case *ir.BuiltinCall:
				if i.Name == "_unlink_jmp_" || i.Name == "_link_jmp_" || strings.Contains(i.Name, "destruct") {
					return true
				}
			}
		}
	}
	return false
}

// isStraightLine checks if the function has single-path execution from entry to return.
func isStraightLine(f *ir.Function) bool {
	if len(f.Blocks) == 0 {
		return false
	}
	if len(f.Blocks) == 1 {
		_, isRet := f.Blocks[0].Terminator.(*ir.Return)
		return isRet
	}
	if len(f.Blocks) == 2 {
		j, isJump := f.Blocks[0].Terminator.(*ir.Jump)
		if !isJump || j.Target != f.Blocks[1] {
			return false
		}
		_, isRet := f.Blocks[1].Terminator.(*ir.Return)
		if !isRet {
			return false
		}
		// f.Blocks[1] must not have real instructions other than markers, terminators, and single-edge phis
		for _, instr := range f.Blocks[1].Instructions {
			if _, isMarker := instr.(*ir.SourceMarker); isMarker {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			if phi, isPhi := instr.(*ir.Phi); isPhi {
				if len(phi.Edges) == 1 && phi.Edges[0].Block == f.Blocks[0] {
					continue
				}
			}
			return false
		}
		return true
	}
	return false
}

// isAcyclic checks if the function's control flow graph contains no back-edges / cycles.
func isAcyclic(f *ir.Function) bool {
	if len(f.Blocks) == 0 {
		return false
	}
	const (
		white = 0 // unvisited
		gray  = 1 // currently visiting (on DFS recursion stack)
		black = 2 // finished
	)
	color := make(map[*ir.BasicBlock]int, len(f.Blocks))
	var dfs func(b *ir.BasicBlock) bool
	dfs = func(b *ir.BasicBlock) bool {
		color[b] = gray
		var succs []*ir.BasicBlock
		switch t := b.Terminator.(type) {
		case *ir.Jump:
			if t.Target != nil {
				succs = append(succs, t.Target)
			}
		case *ir.Branch:
			if t.TrueBlock != nil {
				succs = append(succs, t.TrueBlock)
			}
			if t.FalseBlock != nil {
				succs = append(succs, t.FalseBlock)
			}
		case *ir.Return:
			// Leaf terminator
		default:
			return false // Unknown terminator, reject
		}

		for _, s := range succs {
			c := color[s]
			if c == gray {
				return false // Cycle detected!
			}
			if c == white {
				if !dfs(s) {
					return false
				}
			}
		}
		color[b] = black
		return true
	}

	return dfs(f.Blocks[0])
}

// topologicalSort returns the reachable blocks of f in topological order (definitions before uses).
func topologicalSort(blocks []*ir.BasicBlock) []*ir.BasicBlock {
	if len(blocks) == 0 {
		return nil
	}
	visited := make(map[*ir.BasicBlock]bool, len(blocks))
	var order []*ir.BasicBlock
	var visit func(b *ir.BasicBlock)
	visit = func(b *ir.BasicBlock) {
		if visited[b] {
			return
		}
		visited[b] = true
		switch t := b.Terminator.(type) {
		case *ir.Jump:
			if t.Target != nil {
				visit(t.Target)
			}
		case *ir.Branch:
			if t.TrueBlock != nil {
				visit(t.TrueBlock)
			}
			if t.FalseBlock != nil {
				visit(t.FalseBlock)
			}
		}
		order = append(order, b)
	}
	visit(blocks[0])

	// Reverse order for topological sort
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// canInlineCFG checks if an acyclic multi-block function fits within block and instruction budgets.
func canInlineCFG(f *ir.Function, maxInst int, maxBlocks int, allowCalls bool) bool {
	if len(f.Blocks) == 0 || len(f.Blocks) > maxBlocks {
		return false
	}
	if !isAcyclic(f) {
		return false
	}
	instrCount := 0
	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			switch instr.(type) {
			case *ir.SourceMarker, ir.Terminator, *ir.Phi:
				continue
			case *ir.Call, *ir.IndirectCall:
				if !allowCalls {
					return false
				}
				instrCount += 3
			default:
				instrCount++
			}
		}
	}
	return instrCount <= maxInst
}

// isTinyFunction checks if a function is small enough to inline unconditionally everywhere.
func isTinyFunction(f *ir.Function, maxTiny int) bool {
	if !isStraightLine(f) {
		return false
	}
	instrCount := 0
	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			switch instr.(type) {
			case *ir.SourceMarker, ir.Terminator, *ir.Phi:
				continue
			case *ir.Call, *ir.IndirectCall, *ir.BuiltinCall:
				return false // Tiny functions should not have nested calls
			default:
				instrCount++
			}
		}
	}
	return instrCount <= maxTiny
}

// inlineStraightLine inlines a straight-line callee into caller block b at callIdx.
func inlineStraightLine(caller *ir.Function, b *ir.BasicBlock, callIdx int, call *ir.Call, callee *ir.Function, nextID func() int) {
	valMap := make(map[ir.Value]ir.Value)
	for i, param := range callee.Parameters {
		if i < len(call.Args) {
			valMap[param] = call.Args[i]
		}
	}

	var clonedInstrs []ir.Instruction
	var retVal ir.Value

	for _, cBlk := range callee.Blocks {
		for _, instr := range cBlk.Instructions {
			if _, isMarker := instr.(*ir.SourceMarker); isMarker {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			if phi, isPhi := instr.(*ir.Phi); isPhi {
				if len(phi.Edges) == 1 {
					valMap[phi] = mapVal(phi.Edges[0].Value, valMap)
					continue
				}
			}
			cloned := cloneInstruction(instr, nextID(), valMap, nil)
			valMap[instr] = cloned
			clonedInstrs = append(clonedInstrs, cloned)
		}
		if ret, ok := cBlk.Terminator.(*ir.Return); ok {
			if ret.Val != nil {
				retVal = mapVal(ret.Val, valMap)
			}
		}
	}

	if retVal != nil {
		ReplaceUsesOf(caller, call, retVal)
	}

	newInstructions := make([]ir.Instruction, 0, len(b.Instructions)-1+len(clonedInstrs))
	newInstructions = append(newInstructions, b.Instructions[:callIdx]...)
	newInstructions = append(newInstructions, clonedInstrs...)
	newInstructions = append(newInstructions, b.Instructions[callIdx+1:]...)
	b.Instructions = newInstructions
}

// inlineCFG inlines an acyclic multi-block callee into caller block b at callIdx.
func inlineCFG(
	caller *ir.Function,
	bCaller *ir.BasicBlock,
	callIdx int,
	call *ir.Call,
	callee *ir.Function,
	nextID func() int,
	nextBlockID func() int,
) {
	valMap := make(map[ir.Value]ir.Value)
	for i, param := range callee.Parameters {
		if i < len(call.Args) {
			valMap[param] = call.Args[i]
		}
	}

	sortedCalleeBlocks := topologicalSort(callee.Blocks)
	if len(sortedCalleeBlocks) == 0 {
		return
	}

	blockMap := make(map[*ir.BasicBlock]*ir.BasicBlock, len(sortedCalleeBlocks))
	for _, cBlk := range sortedCalleeBlocks {
		blockMap[cBlk] = &ir.BasicBlock{
			ID: nextBlockID(),
		}
	}

	tailBlk := &ir.BasicBlock{
		ID:           nextBlockID(),
		Instructions: bCaller.Instructions[callIdx+1:],
		Terminator:   bCaller.Terminator,
		Successors:   bCaller.Successors,
	}

	for _, succ := range bCaller.Successors {
		for idx, pred := range succ.Predecessors {
			if pred == bCaller {
				succ.Predecessors[idx] = tailBlk
			}
		}
		for _, instr := range succ.Instructions {
			if phi, ok := instr.(*ir.Phi); ok {
				for idx, edge := range phi.Edges {
					if edge.Block == bCaller {
						phi.Edges[idx].Block = tailBlk
					}
				}
			}
		}
	}

	entryPrime := blockMap[sortedCalleeBlocks[0]]
	bCaller.Instructions = bCaller.Instructions[:callIdx]
	bCaller.Terminator = &ir.Jump{
		BaseInstruction: ir.BaseInstruction{
			ID:      nextID(),
			Typ:     ir.TypeVoid,
			Comment: "inline entry",
		},
		Target: entryPrime,
	}
	bCaller.Successors = []*ir.BasicBlock{entryPrime}
	entryPrime.Predecessors = []*ir.BasicBlock{bCaller}

	type retEdge struct {
		block *ir.BasicBlock
		val   ir.Value
	}
	var retEdges []retEdge

	for _, cBlk := range sortedCalleeBlocks {
		bPrime := blockMap[cBlk]

		for _, instr := range cBlk.Instructions {
			if _, isMarker := instr.(*ir.SourceMarker); isMarker {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			cloned := cloneInstruction(instr, nextID(), valMap, blockMap)
			valMap[instr] = cloned
			bPrime.Instructions = append(bPrime.Instructions, cloned)
		}

		switch t := cBlk.Terminator.(type) {
		case *ir.Jump:
			target := blockMap[t.Target]
			bPrime.Terminator = &ir.Jump{
				BaseInstruction: ir.BaseInstruction{
					ID:  nextID(),
					Typ: ir.TypeVoid,
				},
				Target: target,
			}
			bPrime.Successors = []*ir.BasicBlock{target}
			target.Predecessors = append(target.Predecessors, bPrime)

		case *ir.Branch:
			trueBlk := blockMap[t.TrueBlock]
			falseBlk := blockMap[t.FalseBlock]
			bPrime.Terminator = &ir.Branch{
				BaseInstruction: ir.BaseInstruction{
					ID:  nextID(),
					Typ: ir.TypeVoid,
				},
				Condition:  mapVal(t.Condition, valMap),
				TrueBlock:  trueBlk,
				FalseBlock: falseBlk,
			}
			bPrime.Successors = []*ir.BasicBlock{trueBlk, falseBlk}
			trueBlk.Predecessors = append(trueBlk.Predecessors, bPrime)
			falseBlk.Predecessors = append(falseBlk.Predecessors, bPrime)

		case *ir.Return:
			bPrime.Terminator = &ir.Jump{
				BaseInstruction: ir.BaseInstruction{
					ID:      nextID(),
					Typ:     ir.TypeVoid,
					Comment: "inline ret to tail",
				},
				Target: tailBlk,
			}
			bPrime.Successors = []*ir.BasicBlock{tailBlk}
			tailBlk.Predecessors = append(tailBlk.Predecessors, bPrime)

			var rVal ir.Value
			if t.Val != nil {
				rVal = mapVal(t.Val, valMap)
			}
			retEdges = append(retEdges, retEdge{block: bPrime, val: rVal})

		default:
			log.Panicf("inlineCFG: unhandled terminator type %T", cBlk.Terminator)
		}
	}

	// Splice cloned blocks and tailBlk into caller.Blocks
	bIdx := -1
	for idx, blk := range caller.Blocks {
		if blk == bCaller {
			bIdx = idx
			break
		}
	}
	if bIdx == -1 {
		log.Panicf("inlineCFG: bCaller block %d not found in caller %s", bCaller.ID, caller.Name)
	}

	newBlocks := make([]*ir.BasicBlock, 0, len(caller.Blocks)+len(sortedCalleeBlocks)+1)
	newBlocks = append(newBlocks, caller.Blocks[:bIdx+1]...)
	for _, cBlk := range sortedCalleeBlocks {
		newBlocks = append(newBlocks, blockMap[cBlk])
	}
	newBlocks = append(newBlocks, tailBlk)
	newBlocks = append(newBlocks, caller.Blocks[bIdx+1:]...)
	caller.Blocks = newBlocks

	// Handle return value
	if !call.Type().IsVoid() && len(retEdges) > 0 {
		if len(retEdges) == 1 {
			if retEdges[0].val != nil {
				ReplaceUsesOf(caller, call, retEdges[0].val)
			}
		} else {
			allSame := true
			firstVal := retEdges[0].val
			for _, re := range retEdges[1:] {
				if re.val != firstVal {
					allSame = false
					break
				}
			}
			if allSame && firstVal != nil {
				ReplaceUsesOf(caller, call, firstVal)
			} else {
				phiEdges := make([]ir.PhiEdge, len(retEdges))
				for i, re := range retEdges {
					phiEdges[i] = ir.PhiEdge{
						Block: re.block,
						Value: re.val,
					}
				}
				phi := &ir.Phi{
					BaseInstruction: ir.BaseInstruction{
						ID:      nextID(),
						Typ:     call.Type(),
						Comment: "inlined return join",
					},
					Edges: phiEdges,
				}
				tailBlk.Instructions = append([]ir.Instruction{phi}, tailBlk.Instructions...)
				ReplaceUsesOf(caller, call, phi)
			}
		}
	}
}

func mapVal(v ir.Value, valMap map[ir.Value]ir.Value) ir.Value {
	if v == nil {
		return nil
	}
	if repl, ok := valMap[v]; ok {
		return repl
	}
	return v
}

func cloneInstruction(instr ir.Instruction, newID int, valMap map[ir.Value]ir.Value, blockMap map[*ir.BasicBlock]*ir.BasicBlock) ir.Instruction {
	base := ir.BaseInstruction{
		ID:      newID,
		Typ:     instr.Type(),
		Comment: instr.GetComment(),
		Name:    instr.GetName(),
	}

	switch i := instr.(type) {
	case *ir.ConstByte:
		return &ir.ConstByte{BaseInstruction: base, Val: i.Val}
	case *ir.ConstWord:
		return &ir.ConstWord{BaseInstruction: base, Val: i.Val}
	case *ir.Sizeof:
		return &ir.Sizeof{BaseInstruction: base, TargetTyp: i.TargetTyp}
	case *ir.ConstStruct:
		newFields := make([]ir.Value, len(i.Fields))
		for idx, f := range i.Fields {
			newFields[idx] = mapVal(f, valMap)
		}
		return &ir.ConstStruct{BaseInstruction: base, Fields: newFields}
	case *ir.ConstArray:
		newElts := make([]ir.Value, len(i.Elements))
		for idx, e := range i.Elements {
			newElts[idx] = mapVal(e, valMap)
		}
		return &ir.ConstArray{BaseInstruction: base, Elements: newElts}
	case *ir.Load:
		return &ir.Load{BaseInstruction: base, Global: i.Global}
	case *ir.Store:
		return &ir.Store{BaseInstruction: base, Global: i.Global, Val: mapVal(i.Val, valMap)}
	case *ir.BinaryOp:
		return &ir.BinaryOp{BaseInstruction: base, Op: i.Op, Left: mapVal(i.Left, valMap), Right: mapVal(i.Right, valMap)}
	case *ir.Compare:
		return &ir.Compare{BaseInstruction: base, Op: i.Op, Left: mapVal(i.Left, valMap), Right: mapVal(i.Right, valMap)}
	case *ir.UnaryOp:
		return &ir.UnaryOp{BaseInstruction: base, Op: i.Op, Operand: mapVal(i.Operand, valMap)}
	case *ir.ExtractElement:
		return &ir.ExtractElement{BaseInstruction: base, Array: mapVal(i.Array, valMap), Index: mapVal(i.Index, valMap)}
	case *ir.InsertElement:
		return &ir.InsertElement{BaseInstruction: base, Array: mapVal(i.Array, valMap), Index: mapVal(i.Index, valMap), Val: mapVal(i.Val, valMap)}
	case *ir.ExtractField:
		return &ir.ExtractField{BaseInstruction: base, Struct: mapVal(i.Struct, valMap), FieldIndex: i.FieldIndex}
	case *ir.InsertField:
		return &ir.InsertField{BaseInstruction: base, Struct: mapVal(i.Struct, valMap), FieldIndex: i.FieldIndex, Val: mapVal(i.Val, valMap)}
	case *ir.AddressOfGlobal:
		return &ir.AddressOfGlobal{BaseInstruction: base, Global: i.Global}
	case *ir.AddressOfFunc:
		return &ir.AddressOfFunc{BaseInstruction: base, Func: i.Func}
	case *ir.AddressOfField:
		return &ir.AddressOfField{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap), FieldIndex: i.FieldIndex}
	case *ir.AddressOfElement:
		return &ir.AddressOfElement{BaseInstruction: base, ArrayPtr: mapVal(i.ArrayPtr, valMap), Index: mapVal(i.Index, valMap)}
	case *ir.ExtractFieldPtr:
		return &ir.ExtractFieldPtr{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap), FieldIndex: i.FieldIndex}
	case *ir.InsertFieldPtr:
		return &ir.InsertFieldPtr{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap), FieldIndex: i.FieldIndex, Val: mapVal(i.Val, valMap)}
	case *ir.LoadPtr:
		return &ir.LoadPtr{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap)}
	case *ir.StorePtr:
		return &ir.StorePtr{BaseInstruction: base, Ptr: mapVal(i.Ptr, valMap), Val: mapVal(i.Val, valMap)}
	case *ir.ZeroInit:
		return &ir.ZeroInit{BaseInstruction: base}
	case *ir.Cast:
		return &ir.Cast{BaseInstruction: base, Op: i.Op, Operand: mapVal(i.Operand, valMap)}
	case *ir.Call:
		newArgs := make([]ir.Value, len(i.Args))
		for idx, a := range i.Args {
			newArgs[idx] = mapVal(a, valMap)
		}
		return &ir.Call{BaseInstruction: base, Func: i.Func, Args: newArgs}
	case *ir.IndirectCall:
		newArgs := make([]ir.Value, len(i.Args))
		for idx, a := range i.Args {
			newArgs[idx] = mapVal(a, valMap)
		}
		return &ir.IndirectCall{BaseInstruction: base, FuncPtr: mapVal(i.FuncPtr, valMap), Args: newArgs}
	case *ir.BuiltinCall:
		newArgs := make([]ir.Value, len(i.Args))
		for idx, a := range i.Args {
			newArgs[idx] = mapVal(a, valMap)
		}
		return &ir.BuiltinCall{BaseInstruction: base, Name: i.Name, Args: newArgs}
	case *ir.Phi:
		newEdges := make([]ir.PhiEdge, len(i.Edges))
		for idx, e := range i.Edges {
			targetBlk := e.Block
			if blockMap != nil && blockMap[e.Block] != nil {
				targetBlk = blockMap[e.Block]
			}
			newEdges[idx] = ir.PhiEdge{
				Block: targetBlk,
				Value: mapVal(e.Value, valMap),
			}
		}
		return &ir.Phi{BaseInstruction: base, Edges: newEdges}
	default:
		log.Panicf("cloneInstruction: unhandled instruction type %T", instr)
		return nil
	}
}
