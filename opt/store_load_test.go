package opt

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestStoreLoadForwardingBasic(t *testing.T) {
	// Function:
	//   loc = AddressOfLocal(x)
	//   c1 = 42
	//   StorePtr(loc, c1)
	//   l1 = LoadPtr(loc)    <-- Should be replaced with c1
	//   c2 = 1
	//   res = Add(l1, c2)
	//   return res

	loc := &ir.AddressOfLocal{
		BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord.PointerTo()},
		Local:           &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 100, Typ: ir.TypeWord}, Val: 0},
	}
	c1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 42}
	store := &ir.StorePtr{
		Ptr: loc,
		Val: c1,
	}
	load := &ir.LoadPtr{
		BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord},
		Ptr:             loc,
	}
	c2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 4, Typ: ir.TypeWord}, Val: 1}
	add := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 5, Typ: ir.TypeWord},
		Op:              "add",
		Left:            load,
		Right:           c2,
	}
	ret := &ir.Return{Val: add}

	b0 := &ir.BasicBlock{
		ID:           0,
		Instructions: []ir.Instruction{loc, c1, store, load, c2, add},
		Terminator:   ret,
	}

	fn := &ir.Function{
		Name:   "test_forward",
		Blocks: []*ir.BasicBlock{b0},
	}

	pass := &StoreLoadForwardingPass{}
	changed := pass.Run(fn)

	if !changed {
		t.Fatalf("expected pass to change function")
	}

	// add.Left should now be c1, not load
	if add.Left != c1 {
		t.Errorf("expected add.Left to be c1 (42), got: %v", add.Left)
	}

	// load should have been removed from b0.Instructions
	for _, instr := range b0.Instructions {
		if instr == load {
			t.Errorf("load instruction was not removed from block")
		}
	}
}

func TestStoreLoadForwardingEscaping(t *testing.T) {
	// Function where local address escapes to a function call:
	//   loc = AddressOfLocal(x)
	//   Call(foo, loc)       <-- loc escapes!
	//   c1 = 42
	//   StorePtr(loc, c1)
	//   Call(bar)            <-- Call could mutate loc!
	//   l1 = LoadPtr(loc)    <-- Must NOT be forwarded because call clobbers escaping locals!
	//   return l1

	fnFoo := &ir.Function{Name: "foo"}
	fnBar := &ir.Function{Name: "bar"}

	loc := &ir.AddressOfLocal{
		BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord.PointerTo()},
		Local:           &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 100, Typ: ir.TypeWord}, Val: 0},
	}
	callFoo := &ir.Call{
		Func: fnFoo,
		Args: []ir.Value{loc},
	}
	c1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 42}
	store := &ir.StorePtr{
		Ptr: loc,
		Val: c1,
	}
	callBar := &ir.Call{
		Func: fnBar,
	}
	load := &ir.LoadPtr{
		BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord},
		Ptr:             loc,
	}
	ret := &ir.Return{Val: load}

	b0 := &ir.BasicBlock{
		ID:           0,
		Instructions: []ir.Instruction{loc, callFoo, c1, store, callBar, load},
		Terminator:   ret,
	}

	fn := &ir.Function{
		Name:   "test_escaping",
		Blocks: []*ir.BasicBlock{b0},
	}

	pass := &StoreLoadForwardingPass{}
	changed := pass.Run(fn)

	if changed {
		t.Fatalf("escaping local should NOT have been forwarded across call")
	}

	if ret.Val != load {
		t.Errorf("ret.Val should still be load")
	}
}
