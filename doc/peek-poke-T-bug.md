Tell me more about the bug in this comment you made, and how it should
be fixed:

``` While we couldn't use `peek[\*byte]` because of a bug with generic
type substitution for pointer types (it expands to `\*\*byte` internally
and throws a `PointedType called on non-pointer type: byte` error) ```

3:46 PM

The bug you're seeing stems from how Minigolf's compiler handles generic
function instantiation (specifically, the substituteGenericTokens function
in ir/builder.go).

Instead of operating strictly on the Abstract Syntax Tree (AST),
the Minigolf compiler implements generic substitution at the lexical
token level.

Here is exactly what happens step-by-step:

When you call peek[\*byte](x.BaseAddr), the compiler looks at the type
argument \*byte.

It converts \*byte back into a stream of raw lexer tokens: \* and byte.

It takes the token stream for the generic body return \*((\*T)(addr))
and replaces the token T with the two tokens \* and byte.

The resulting token stream becomes return \*( (\* \* byte) (addr) ).

The compiler then feeds this new token stream back into the parser to
generate a new AST for the instantiated function.

The Breakdown

Because of those consecutive \* tokens, the parser misinterprets the
structure. In Go/Minigolf, an \* can mean either a "pointer type" or a
"dereference operation".

The parser sees \* \* byte and builds an AST with deeply nested
\*ast.PointerType nodes (essentially a pointer to a pointer to a byte).

Later, during the IR building phase (buildExpr), the compiler tries to
evaluate this AST. When it processes the \*ast.PointerType, it attempts to
find out what type is being pointed to by calling .PointedType(). Because
the AST nodes were munged during the lexical token substitution, the
compiler accidentally drills too deep and calls .PointedType() on the
base byte identifier itself.

Since byte isn't a pointer, it triggers the panic: PointedType called
on non-pointer type: byte

How it should be fixed The architectural fix would be to move generic
substitution from the Lexical phase to the AST phase.

Instead of injecting raw tokens and re-parsing, instantiateGenericFunc
should traverse the already-parsed generic AST and replace \*ast.Identifier
nodes named T with the resolved Type's AST nodes (in this case, an
\*ast.PointerType wrapping a byte).

This ensures that parentheses, type contexts, and dereference operations
are strictly preserved and the parser doesn't get confused by
consecutive \* tokens.
