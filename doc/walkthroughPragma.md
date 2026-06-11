# Pragma Comment Support Implementation

We added support for inline pragma comments (e.g. `// minigolf: CHECK_BOUNDS=1 CHECK_NIL=0`) to enable developers to customize compiler behavior, explicitly overriding environment variables and command-line flags. 

## Changes Made

### 1. Lexer & Token Modifications
- Added `token.PRAGMA` constant to `token/token.go`.
- In `lexer/lexer.go`, intercepted single-line `//` comments. Before skipping the comment, we now extract its text. If the text matches `// minigolf: ...`, we parse the ensuing key-value pairs and return a `PRAGMA` token to the parser.

### 2. AST & Parser Enhancements 
- Added an `ast.PragmaStatement` definition to `ast/ast.go`.
- Added a `parsePragmaStatement()` handler to the statement parser in `parser/parser.go`, effectively injecting pragmas directly into the AST seamlessly alongside variable declarations, functions, and standard logic.

### 3. Semantic Overrides
- Expanded `semantic.Analyzer` with a `Pragmas map[string]string` registry.
- During semantic analysis, we capture all top-level pragmas and hydrate the config map.
- Updated `main.go` to introspect `analyzer.Pragmas` and overwrite `*checkBoundsFlag` and `*checkNilFlag` where applicable just before launching into Backend generation code.

## Verification
Created `tests/pragma1.golf` & `tests/pragma2.golf`. Both files include `// minigolf: CHECK_BOUNDS=1 CHECK_NIL=1` at the very beginning and initiate intentional panics via uninitialized struct deference and out-of-bounds slice accesses respectively. Using `go run main.go -m=cbe` without explicit environment flags resulted in predictable crashes, successfully proving the compilation overrides functioned.
