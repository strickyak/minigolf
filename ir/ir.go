package ir

import "fmt"

// Type represents a primitive type in the IR.
type Type int

const (
	TypeUnknown Type = iota
	TypeByte
	TypeWord
	TypeVoid
)

func (t Type) String() string {
	switch t {
	case TypeByte:
		return "byte"
	case TypeWord:
		return "word"
	case TypeVoid:
		return "void"
	}
	return "unknown"
}

// Value is an interface for anything that can be an operand.
type Value interface {
	Type() Type
	String() string
}

// Program represents the entire compilation unit in SSA form.
type Program struct {
	Globals   []*Global
	Functions []*Function
}

// Function represents a single function in SSA form.
type Function struct {
	Name       string
	Parameters []*Parameter
	ReturnType Type
	Blocks     []*BasicBlock
}

// Parameter represents a function parameter.
type Parameter struct {
	ID   int
	Name string
	Typ  Type
}

func (p *Parameter) Type() Type       { return p.Typ }
func (p *Parameter) String() string   { return fmt.Sprintf("p%d", p.ID) }

// BasicBlock is a sequence of non-branching instructions ending in a terminator.
type BasicBlock struct {
	ID           int
	Instructions []Instruction
	Terminator   Terminator
	Predecessors []*BasicBlock
	Successors   []*BasicBlock
}

// Global represents a global variable in the flat namespace.
type Global struct {
	Name string
	Typ  Type
}

func (g *Global) Type() Type       { return g.Typ }
func (g *Global) String() string   { return fmt.Sprintf("@%s", g.Name) }

// StringLiteral represents a string constant (mostly for print builtins).
type StringLiteral struct {
	Value string
}

func (s *StringLiteral) Type() Type       { return TypeUnknown }
func (s *StringLiteral) String() string   { return fmt.Sprintf("%q", s.Value) }

// Instruction is the interface for all SSA operations.
type Instruction interface {
	Value
	Opcode() string
	SetID(int)
	GetID() int
}

// Terminator represents an instruction that safely ends a BasicBlock.
type Terminator interface {
	Instruction
	IsTerminator()
}

// BaseInstruction provides common Instruction fields.
type BaseInstruction struct {
	ID  int
	Typ Type
}

func (b *BaseInstruction) Type() Type       { return b.Typ }
func (b *BaseInstruction) String() string   { return fmt.Sprintf("v%d", b.ID) }
func (b *BaseInstruction) SetID(id int)     { b.ID = id }
func (b *BaseInstruction) GetID() int       { return b.ID }

// --- Constant Instructions ---

type ConstByte struct {
	BaseInstruction
	Val uint8
}
func (i *ConstByte) Opcode() string { return "const_byte" }

type ConstWord struct {
	BaseInstruction
	Val uint64
}
func (i *ConstWord) Opcode() string { return "const_word" }

// --- Memory Operations ---

type Load struct {
	BaseInstruction
	Global *Global
}
func (i *Load) Opcode() string { return "load" }

type Store struct {
	BaseInstruction
	Global *Global
	Val    Value
}
func (i *Store) Opcode() string { return "store" }

// --- Arithmetic & Logic Operations ---

type BinaryOp struct {
	BaseInstruction
	Op    string // "add", "sub", "mul", "div", "mod", "and", "or", "xor", "shl", "shr"
	Left  Value
	Right Value
}
func (i *BinaryOp) Opcode() string { return i.Op }

type Compare struct {
	BaseInstruction // Comparison returns a byte (0 or 1)
	Op    string // "eq", "neq", "lt", "lte", "gt", "gte"
	Left  Value
	Right Value
}
func (i *Compare) Opcode() string { return i.Op }

type UnaryOp struct {
	BaseInstruction
	Op      string // "not", "neg"
	Operand Value
}
func (i *UnaryOp) Opcode() string { return i.Op }

// --- SSA Primitives ---

type Phi struct {
	BaseInstruction
	Edges []PhiEdge
}
type PhiEdge struct {
	Block *BasicBlock
	Value Value
}
func (i *Phi) Opcode() string { return "phi" }

// --- Function Calls ---

type Call struct {
	BaseInstruction
	Func *Function
	Args []Value
}
func (i *Call) Opcode() string { return "call" }

type BuiltinCall struct {
	BaseInstruction
	Name string // "print", "println"
	Args []Value
}
func (i *BuiltinCall) Opcode() string { return "builtin_" + i.Name }

// --- Type Conversions ---

type Cast struct {
	BaseInstruction
	Op      string // "zero_ext", "trunc"
	Operand Value
}
func (i *Cast) Opcode() string { return i.Op }

// --- Terminators ---

type Jump struct {
	BaseInstruction
	Target *BasicBlock
}
func (i *Jump) Opcode() string { return "jmp" }
func (i *Jump) IsTerminator() {}

type Branch struct {
	BaseInstruction
	Condition  Value
	TrueBlock  *BasicBlock
	FalseBlock *BasicBlock
}
func (i *Branch) Opcode() string { return "br" }
func (i *Branch) IsTerminator() {}

type Return struct {
	BaseInstruction
	Val Value // nil if void
}
func (i *Return) Opcode() string { return "ret" }
func (i *Return) IsTerminator() {}
