# Fold cc_to_golf into minigolf

## Background

`cc_to_golf` is currently a standalone `package main` binary. The plan folds it
into `minigolf` as a proper sub-package so that one binary handles both C and
MiniGolf source files.

---

## Proposed Changes

### Component 1 — New package `cc_v5/translator`

Refactor `cc_v5/cmd/cc_to_golf/cc_to_golf.go` from `package main` into a
reusable library package.

#### [NEW] `cc_v5/translator/translator.go`

Move the entire `translator` struct and all its methods here.
Change `package main` → `package translator`.

Remove the `keepGoing` global flag. Replace it with a field on the `translator`
struct so callers can control it:

```go
type translator struct {
    KeepGoing bool   // emit /* unsupported */ comments instead of panicking
    // ... existing fields ...
}
```

Add a clean public API:

```go
// Options controls the translation.
type Options struct {
    // IncludePaths is searched for #include "..." directives (in order).
    // The last entry is also used for #include <...>.
    IncludePaths []string
    // Defines maps C preprocessor macro names (no dot) to their values.
    // These are injected as extra #define lines before the source.
    Defines map[string]string
    // KeepGoing emits /* unsupported */ comments instead of panicking.
    KeepGoing bool
}

// TranslateFile reads a C source file and returns the MiniGolf source string.
func TranslateFile(cFile string, opts Options) (string, error)
```

**Config construction** (no `NewConfig` by default):

```go
func buildConfig(opts Options) (*cc.Config, error) {
    cfg, err := cc.NewConfig(runtime.GOOS, runtime.GOARCH)
    if err != nil {
        return nil, err
    }
    cfg.Predefined += cc.Builtin
    // Add user include paths.
    if len(opts.IncludePaths) > 0 {
        cfg.IncludePaths = append([]string{"@"}, opts.IncludePaths...)
        // The final -I directory is the system include path (for <...>).
        cfg.SysIncludePaths = []string{opts.IncludePaths[len(opts.IncludePaths)-1]}
    }
    return cfg, nil
}
```

**Defines injection**: prepend a synthetic `Source` with `#define NAME value`
lines for each entry in `opts.Defines` (those without a dot in the name).

#### [DELETE or thin] `cc_v5/cmd/cc_to_golf/cc_to_golf.go`

Keep as a tiny shim `package main` that delegates to `translator.TranslateFile`:

```go
package main

import (
    "flag"; "fmt"; "os"; "strings"
    "github.com/strickyak/minigolf/cc_v5/translator"
)

var keepGoing = flag.Bool("k", false, "keep going on unsupported constructs")

func main() {
    flag.Parse()
    args := flag.Args()
    if len(args) < 1 { ... }
    golf, err := translator.TranslateFile(args[0], translator.Options{
        KeepGoing: *keepGoing,
    })
    if err != nil { fmt.Fprintln(os.Stderr, err) }
    fmt.Print(golf)
}
```

---

### Component 2 — `main.go` changes

#### Flag additions / changes

```
-m cc_to_golf   stop after C→Golf translation, write .golf intermediate
                (uppercase CC_TO_GOLF also accepted, like AST/IR)
```

**`-D` flag routing** (split on presence of `.`):
```
-Dfoo=bar        → C preprocessor: #define foo bar      (no dot → CPP)
-Dfoo.bar=baz    → MiniGolf constant override           (has dot → existing path)
```

**`-I` flag**: already exists as `importDirPath repeatedFlag`. It will now
also be forwarded to `translator.Options.IncludePaths` when compiling `.c`.
The last `-I` directory is treated as the system include path for `#include <...>`.

No new flag is needed.

#### Source file detection

```go
mainSourceFile := sourceFiles[0]
isC := strings.HasSuffix(mainSourceFile, ".c")
```

#### C→Golf translation step (new, before `ParseSourceFiles`)

```go
if isC {
    cDefines := map[string]string{}
    for name, val := range defines {
        if !strings.Contains(name, ".") {
            cDefines[name] = val   // route to CPP
        }
    }
    golfSrc, err := translator.TranslateFile(mainSourceFile, translator.Options{
        IncludePaths: []string(importDirPath),
        Defines:      cDefines,
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "C translation error: %v\n", err)
        os.Exit(1)
    }

    // Write the intermediate .golf file.
    if *outFlag != "" {
        tmpGolf := *outFlag + ".tmp.golf"
        if err := os.WriteFile(tmpGolf, []byte(golfSrc), 0644); err != nil {
            fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", tmpGolf, err)
            os.Exit(1)
        }
        mainSourceFile = tmpGolf
    } else {
        // No -o: write Golf source to stderr for debugging.
        fmt.Fprint(os.Stderr, golfSrc)
        // Still need a temp file for the parser.
        f, _ := os.CreateTemp("", "*.tmp.golf")
        f.WriteString(golfSrc)
        f.Close()
        mainSourceFile = f.Name()
        defer os.Remove(mainSourceFile)
    }

    // -m=cc_to_golf → stop here, translation is the only output.
    if strings.ToUpper(*archFlag) == "CC_TO_GOLF" {
        os.Exit(0)
    }
}
```

#### `-D` routing

```go
golfDefines := make(map[string]string)
for _, d := range defineFlags {
    parts := strings.SplitN(d, "=", 2)
    // ... existing validation ...
    if strings.Contains(parts[0], ".") {
        golfDefines[parts[0]] = parts[1]   // MiniGolf constant (existing path)
    }
    // CPP defines are handled inside translator.TranslateFile (see above)
}
// Pass golfDefines to semantic.NewResolver (was `defines` before)
resolver := semantic.NewResolver(golfDefines)
```

---

### Component 3 — `c_test.go` update

#### [MODIFY] `c_test.go`

Replace the `cc_to_golf` subprocess invocation with an in-process call:

```go
import "github.com/strickyak/minigolf/cc_v5/translator"

// Inside the test loop:
golfSrc, err := translator.TranslateFile(cFile, translator.Options{
    IncludePaths: []string{"golflib"},
})
```

Remove the `ccToGolf` binary build step entirely.

---

### Component 4 — `golflib/` C headers

Create `golflib/minigolf.h` as the standard MiniGolf C preamble:

```c
/* minigolf.h — Standard MiniGolf C preamble.
 * Include this at the top of any C file compiled with minigolf -m=<arch> file.c
 */
#ifndef MINIGOLF_H
#define MINIGOLF_H

extern void putchar(char ch);
typedef unsigned char  byte;
typedef unsigned long  word;

#endif /* MINIGOLF_H */
```

Users can then write:
```c
#include "minigolf.h"   /* or #include <minigolf.h> if golflib is last -I */
```

---

### Component 5 — `-m=NewConfig` bootstrap mode

> [!NOTE]
> Deferred / optional. The `-m=NewConfig` mode would invoke `cc.NewConfig` and
> print the discovered predefined macros and include paths to stdout — useful
> for capturing a host config to later bake in as a static config. This is a
> developer/porting tool and need not be implemented in the initial integration.

---

## Verification Plan

### Automated tests

1. `go build ./...` — ensure the new `translator` package compiles cleanly.
2. `go test -run TestAllCFiles` — all existing C tests pass via the in-process
   translator (no subprocess).
3. `go test -run TestSystemAllGolfFiles` — ensure no regressions in Golf path.

### Manual verification

```sh
# Test -m=cc_to_golf stop mode
minigolf -m=cc_to_golf -o /tmp/test c-tests/hello1.c
cat /tmp/test.tmp.golf     # should contain the translated Golf source

# Test full pipeline from .c
minigolf -m=m6809 -o /tmp/hello.asm -I golflib c-tests/hello1.c
cat /tmp/hello.tmp.golf    # intermediate visible
# run the .asm

# Test -D routing
minigolf -m=m6809 -DDEBUG=1 -Dfoo.bar=42 -o /tmp/out.asm -I golflib src.c
# DEBUG=1 → CPP #define; foo.bar=42 → MiniGolf constant
```

## Open Questions

> [!IMPORTANT]
> **Should `.tmp.golf` be cleaned up automatically?** If `-o out.asm` is
> given, `out.asm.tmp.golf` is a side-effect file. Should it be removed after
> a successful compile, or kept for inspection? Keeping it seems more useful
> during development.

> [!IMPORTANT]
> **`NewConfig` cost**: `cc.NewConfig` spawns a subprocess (`cc -dM -E -`) to
> harvest predefined macros. This is ~100–200ms. For the initial implementation
> it is acceptable. A future `-m=NewConfig` bootstrap mode could cache the
> result. Is the latency acceptable for now?

> [!NOTE]
> **ABI mismatch**: `cc.NewConfig` uses host ABI sizes (64-bit pointers on
> x86_64). For M6809 (16-bit pointers), type sizes like `sizeof(long)` differ.
> For our simple tests this doesn't matter because we use `byte`/`word` typedefs.
> A proper fix requires a custom `Config.ABI` — deferred.
