package opt

import "github.com/strickyak/minigolf/ir"

// MagicFuncNames lists every prelude function that the IR builder may call
// *implicitly* while lowering an operator expression, without any explicit
// call-site in user code or generic-template code.
//
// Because these calls only appear after the IR is built (after all templates
// have been expanded), they are invisible to the AST-level dead-function
// trimmer and to early IR-level DCE.  Both passes must treat magic functions
// as unconditionally reachable during their first ("normal") round.
//
// A final ("magic") DCE round — run after IR building and all optimizations
// — may then remove any magic function whose IsMagic flag is set and that is
// still unreachable via the normal call graph.
//
// When adding a new implicit operator helper to ir/builder.go, add its fully-
// qualified prelude name here as well.
var MagicFuncNames = []string{
	// String comparison (used for '<', '<=', '>', '>=' on strings)
	"prelude.strcmp",
	// String equality (used for '==', '!=' on strings)
	"prelude.streq",
	// Struct/array memory equality (used for '==', '!=' on structs/arrays)
	"prelude.memeq",
	// Raw memory compare (called by strcmp, memeq, etc.)
	"prelude.memcmp",
	// Word arithmetic helpers needed on 16-bit targets (WordSize == 2)
	"prelude.mul_word",
	"prelude.mul_byte",
	"prelude.div_word",
	"prelude.mod_word",
}

// MagicFuncSet is a fast-lookup set derived from MagicFuncNames.
var MagicFuncSet = func() map[string]bool {
	m := make(map[string]bool, len(MagicFuncNames))
	for _, name := range MagicFuncNames {
		m[name] = true
	}
	return m
}()

// MarkMagicFunctions sets IsMagic=true on every function in the IR program
// whose fully-qualified name is in MagicFuncNames.  Call this immediately
// after ir.Builder.Build() so that EliminateDeadFunctions can use the flag.
func MarkMagicFunctions(p *ir.Program) {
	for _, f := range p.Functions {
		if MagicFuncSet[f.Name] {
			f.IsMagic = true
		}
	}
}
