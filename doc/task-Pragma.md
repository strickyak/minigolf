# Implement Pragmas

- `[x]` Add `token.PRAGMA` to `token/token.go`
- `[x]` Update `lexer/lexer.go` to parse `// minigolf: ...` and emit `token.PRAGMA`
- `[x]` Add `PragmaStatement` to `ast/ast.go`
- `[x]` Update `parser/parser.go` to parse `token.PRAGMA` into `PragmaStatement`
- `[x]` Update `semantic/semantic.go` to collect pragmas
- `[x]` Update `main.go` to override flags using `analyzer.Pragmas`
- `[x]` Test pragmas with bounds and nil checking
