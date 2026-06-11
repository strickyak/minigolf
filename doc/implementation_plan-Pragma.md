# Pragma Implementation Plan

We will add support for Go-style pragma comments, specifically parsing comments prefixed with `// minigolf:` into a synthetic `pragma` AST node.

## Proposed Changes

### Lexer and Token
#### [MODIFY] [token/token.go](file:///home/strick/antig/token/token.go)
- Add a new token type: `PRAGMA = "PRAGMA"`

#### [MODIFY] [lexer/lexer.go](file:///home/strick/antig/lexer/lexer.go)
- In the `case '/'` block, intercept `//` single-line comments. Before skipping the rest of the line, capture the comment text.
- If the comment begins with `// minigolf:`, extract the payload, emit a `token.PRAGMA` token, and return it.

### AST and Parser
#### [MODIFY] [ast/ast.go](file:///home/strick/antig/ast/ast.go)
- Add a `PragmaStatement` struct to represent the pragma in the AST. Include the `Token` and `Value` (string).

#### [MODIFY] [parser/parser.go](file:///home/strick/antig/parser/parser.go)
- In `parseStatement`, add a case for `token.PRAGMA` that calls `parsePragmaStatement()`.
- `parsePragmaStatement()` consumes the token, optionally consumes a trailing semicolon (though unlikely given ASI rules), and returns the `*ast.PragmaStatement`.

### Semantic Phase and Compilation State
#### [MODIFY] [semantic/semantic.go](file:///home/strick/antig/semantic/semantic.go)
- Add a `Pragmas map[string]string` field to `Analyzer`.
- During the initial AST traversal in `Analyze` (Pass 1), check for `*ast.PragmaStatement` nodes.
- When found, parse the string (e.g., `CHECK_BOUNDS=1 CHECK_NIL=0`) into key-value pairs and store them in `a.Pragmas`.

#### [MODIFY] [main.go](file:///home/strick/antig/main.go)
- Following the semantic analysis phase `analyzer.Analyze(program)`, inspect `analyzer.Pragmas`.
- If `CHECK_BOUNDS` is present, override `*checkBoundsFlag`.
- If `CHECK_NIL` is present, override `*checkNilFlag`.
- This efficiently pushes the overridden flags directly into `builder.CheckBounds` and `builder.CheckNil` as established previously.

## Verification Plan

### Automated Tests
- I will create a test `tests/pragma_test.golf` with `// minigolf: CHECK_BOUNDS=1 CHECK_NIL=1` at the top.
- Without passing environment variables or CLI flags, the compilation should inherently enable bounds and nil checking due to the semantic override.
- I will verify this by invoking an out-of-bounds array access and ensuring a panic triggers.
