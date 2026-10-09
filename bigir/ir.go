package bigir

import (
	"fmt"
)

// Value is anything that can produce a typed value or be an operand.
type Value interface {
	Type() Type
	String() string
}

// Instruction is the interface for all BIGIR instructions.
type Instruction interface {
	Value
	Opcode() string
	SetID(int)
	GetID() int
	GetComment() string
	SetComment(string)
}

// Terminator is an instruction that safely ends a BasicBlock.
type Terminator interface {
	Instruction
	isTerminator()
}

// BaseInstruction provides shared instruction fields.
type BaseInstruction struct {
	ID      int
	Typ     Type
	Comment string
}

func (b *BaseInstruction) Type() Type          { return b.Typ }
func (b *BaseInstruction) SetID(id int)        { b.ID = id }
func (b *BaseInstruction) GetID() int          { return b.ID }
func (b *BaseInstruction) GetComment() string  { return b.Comment }
func (b *BaseInstruction) SetComment(c string) { b.Comment = c }
func (b *BaseInstruction) String() string      { return fmt.Sprintf("v%d", b.ID) }

// Parameter represents a function argument.
type Parameter struct {
	ID   int
	Name string
	Typ  Type
}

func (p *Parameter) Type() Type     { return p.Typ }
func (p *Parameter) String() string { return fmt.Sprintf("p%d_%s", p.ID, p.Name) }

// BasicBlock is a linear sequence of instructions ending in a terminator.
type BasicBlock struct {
	ID           int
	Label        string
	Instructions []Instruction
	Terminator   Terminator
	Predecessors []*BasicBlock
	Successors   []*BasicBlock
}

func (bb *BasicBlock) String() string {
	if bb.Label != "" {
		return fmt.Sprintf("bb%d_%s", bb.ID, bb.Label)
	}
	return fmt.Sprintf("bb%d", bb.ID)
}

// Global represents a global variable in fixed memory or Far Data.
type Global struct {
	Name       string
	Typ        Type
	IsFar      bool   // true if placed in Far Data block; false if in Slot 0/6
	InitString string // For string literals
	InitVal    Value
}

func (g *Global) Type() Type     { return g.Typ }
func (g *Global) String() string { return "@" + g.Name }

// Trampoline represents an entry stub in Slot 6 for calling a Far Function in Slot 5.
type Trampoline struct {
	FuncName    string
	TargetBlock int    // Physical block ID (8..127)
	TargetAddr  uint16 // Virtual address in Slot 5 ($A000..$BFFF)
}

// Function represents a compiled procedure in BIGIR.
type Function struct {
	Name         string
	IsFar        bool   // true for user code (executes in Slot 5); false for fixed runtime (Slot 6)
	BlockID      int    // Assigned physical block ID (8..127 for Far; 6 for fixed runtime)
	Slot5Offset  uint16 // Offset within Slot 5 ($A000..$BFFF)
	Parameters   []*Parameter
	ReturnType   Type
	Blocks       []*BasicBlock
	EntryBlock   *BasicBlock
	AllocSize    int    // Estimated code size in bytes
}

// Program is the complete compilation unit in BIGIR.
type Program struct {
	Globals      []*Global
	Functions    []*Function
	FarBlocks    map[int][]*Function // Physical block ID -> functions assigned to that block
	Trampolines  []*Trampoline
	TypeDefs     map[string]Type
}

// --- Constant Instructions ---

type ConstByte struct {
	BaseInstruction
	Val uint8
}
func (c *ConstByte) Opcode() string { return "const_byte" }
func (c *ConstByte) String() string { return fmt.Sprintf("%d", c.Val) }

type ConstWord struct {
	BaseInstruction
	Val uint64
}
func (c *ConstWord) Opcode() string { return "const_word" }
func (c *ConstWord) String() string { return fmt.Sprintf("%d", c.Val) }

type ConstFarRef struct {
	BaseInstruction
	BlockID  uint8  // 128..255
	ChunkIdx uint16 // 0..511
}
func (c *ConstFarRef) Opcode() string { return "const_far_ref" }
func (c *ConstFarRef) String() string { return fmt.Sprintf("FarRef(%d:%d)", c.BlockID, c.ChunkIdx) }

type ConstString struct {
	BaseInstruction
	Val      string
	GlobalRef *Global
}
func (c *ConstString) Opcode() string { return "const_string" }
func (c *ConstString) String() string { return fmt.Sprintf("%q", c.Val) }

type ConstStruct struct {
	BaseInstruction
	Fields []Value
}
func (c *ConstStruct) Opcode() string { return "const_struct" }
func (c *ConstStruct) String() string { return "const_struct" }

type ConstArray struct {
	BaseInstruction
	Elements []Value
}
func (c *ConstArray) Opcode() string { return "const_array" }
func (c *ConstArray) String() string { return "const_array" }

type FuncRef struct {
	BaseInstruction
	FuncName string
}
func (f *FuncRef) Opcode() string { return "func_ref" }
func (f *FuncRef) String() string { return "&" + f.FuncName }

// --- Arithmetic & Logical Operations ---

type BinaryOp struct {
	BaseInstruction
	Op    string // "add", "sub", "mul", "div", "mod", "and", "or", "xor", "shl", "shr"
	Left  Value
	Right Value
}
func (b *BinaryOp) Opcode() string { return b.Op }

type UnaryOp struct {
	BaseInstruction
	Op      string // "neg", "not"
	Operand Value
}
func (u *UnaryOp) Opcode() string { return u.Op }

type Compare struct {
	BaseInstruction
	Op    string // "eq", "neq", "lt", "lte", "gt", "gte"
	Left  Value
	Right Value
}
func (c *Compare) Opcode() string { return c.Op }

// --- SSA Phi Instruction ---

type PhiEdge struct {
	Block *BasicBlock
	Value Value
}

type Phi struct {
	BaseInstruction
	Edges []PhiEdge
}
func (p *Phi) Opcode() string { return "phi" }

// --- Memory Operations ---

type NearLoad struct {
	BaseInstruction
	Addr Value // 16-bit virtual address (stack, global, or fixed RAM)
}
func (l *NearLoad) Opcode() string { return "near_load" }

type NearStore struct {
	BaseInstruction
	Addr Value
	Val  Value
}
func (s *NearStore) Opcode() string { return "near_store" }

type FarLoad struct {
	BaseInstruction
	FarRef Value // 16-bit FarRef (Block + Chunk)
	Offset Value // 16-bit byte offset
}
func (l *FarLoad) Opcode() string { return "far_load" }

type FarStore struct {
	BaseInstruction
	FarRef Value
	Offset Value
	Val    Value
}
func (s *FarStore) Opcode() string { return "far_store" }

type AddressOfGlobal struct {
	BaseInstruction
	Global *Global
}
func (a *AddressOfGlobal) Opcode() string { return "addrof_global" }

type AddressOfLocal struct {
	BaseInstruction
	Local Value
}
func (a *AddressOfLocal) Opcode() string { return "addrof_local" }

// --- 8-Byte Slice Operations ---

type SliceMake struct {
	BaseInstruction // Typ is KindFarSlice or KindFarString
	FarRef   Value  // 16-bit FarRef (0 if near literal / fixed RAM)
	Offset   Value  // 16-bit byte offset or near virtual address
	Length   Value  // 16-bit element count
	Capacity Value  // 16-bit capacity count
}
func (s *SliceMake) Opcode() string { return "slice_make" }

type SliceGet struct {
	BaseInstruction
	Slice Value // 8-byte slice
	Index Value // 16-bit index
}
func (s *SliceGet) Opcode() string { return "slice_get" }

type SlicePut struct {
	BaseInstruction
	Slice Value
	Index Value
	Val   Value
}
func (s *SlicePut) Opcode() string { return "slice_put" }

type SliceChop struct {
	BaseInstruction
	Slice Value
	Start Value
	Limit Value
}
func (s *SliceChop) Opcode() string { return "slice_chop" }

type SliceField struct {
	BaseInstruction
	Slice     Value
	FieldIdx  int // 0: far_ref, 1: offset, 2: length, 3: capacity
}
func (s *SliceField) Opcode() string { return "slice_field" }

type SliceToPtr struct {
	BaseInstruction
	Slice Value
}
func (s *SliceToPtr) Opcode() string { return "slice_to_ptr" }

type BitCast struct {
	BaseInstruction
	Operand Value
}
func (b *BitCast) Opcode() string { return "bitcast" }


// --- General Struct Operations ---

type ZeroInit struct {
	BaseInstruction
}
func (z *ZeroInit) Opcode() string { return "zero_init" }

type ExtractField struct {
	BaseInstruction
	Struct     Value
	FieldIndex int
	ByteOffset int
	FieldSize  int
}
func (e *ExtractField) Opcode() string { return "extract_field" }

type InsertField struct {
	BaseInstruction
	Struct     Value
	FieldIndex int
	ByteOffset int
	FieldSize  int
	Val        Value
}
func (i *InsertField) Opcode() string { return "insert_field" }

// --- Call Operations ---

type NearCall struct {
	BaseInstruction
	Callee string
	Args   []Value
}
func (c *NearCall) Opcode() string { return "near_call" }

type FarCall struct {
	BaseInstruction
	Callee      string
	TargetBlock int
	TargetAddr  uint16
	Args        []Value
}
func (c *FarCall) Opcode() string { return "far_call" }

type IndirectCall struct {
	BaseInstruction
	FuncPtr Value
	Args    []Value
}
func (c *IndirectCall) Opcode() string { return "indirect_call" }

// --- Terminators ---

type Return struct {
	BaseInstruction
	Val Value // nil for void
}
func (r *Return) Opcode() string  { return "return" }
func (r *Return) isTerminator()   {}

type FarReturn struct {
	BaseInstruction
	Val Value
}
func (r *FarReturn) Opcode() string { return "far_return" }
func (r *FarReturn) isTerminator()  {}

type Branch struct {
	BaseInstruction
	Target *BasicBlock
}
func (b *Branch) Opcode() string  { return "br" }
func (b *Branch) isTerminator()   {}

type CondBranch struct {
	BaseInstruction
	Cond        Value
	TrueTarget  *BasicBlock
	FalseTarget *BasicBlock
}
func (c *CondBranch) Opcode() string { return "cbr" }
func (c *CondBranch) isTerminator()  {}

type Panic struct {
	BaseInstruction
	Message string
}
func (p *Panic) Opcode() string  { return "panic" }
func (p *Panic) isTerminator()   {}
