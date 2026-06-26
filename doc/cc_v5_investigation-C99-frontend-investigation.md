# cc/v5 C99 Frontend — Investigation

## 1. Does cc/v5 have a built-in C Preprocessor?

**Yes — fully built-in.** The `cc/v5` library contains its own complete CPP
implementation (`newCPP`, `preprocess`, etc. in `cc.go`). It handles all
standard `#` directives natively:

| Directive | Handled by |
|-----------|-----------|
| `#define` / `#undef` | Internal CPP |
| `#if` / `#ifdef` / `#ifndef` / `#elif` / `#else` / `#endif` | Internal CPP |
| `#include "file.h"` | CPP → searches `IncludePaths` |
| `#include <file.h>` | CPP → searches `SysIncludePaths` |
| `#pragma` | Forwarded to optional `Config.PragmaHandler` callback |
| `#error` | CPP |
| Predefined macros (`__FILE__`, `__LINE__`, etc.) | Populated from host compiler |

The three entry points are:
- `cc.Preprocess(cfg, sources, writer)` — CPP only, writes text
- `cc.Parse(cfg, sources)` — CPP + parse, returns `*AST`
- `cc.Translate(cfg, sources)` — CPP + parse + type-check, returns typed `*AST`

## 2. Does it need an external C compiler?

**Partially.** `NewConfig()` invokes the host `cc`/`gcc` binary **once at
startup** for two purposes only:

1. **Predefined macros** — runs `cc -dM -E -` to harvest the full set of
   `#define __ARCH__`, `__SIZEOF_LONG__`, platform ABI sizes, etc.
2. **System include paths** — runs `cc -v -E -` and parses the
   `#include <...> search starts here:` block to learn where `<stdio.h>` etc.
   live on the host.

After that, **all** preprocessing and parsing is done in pure Go. The host
compiler is never invoked again. Look-up order: `$CC` env var → `cc` → `gcc`.

### Implication for MiniGolf targets

MiniGolf targets (M6809, CBE) are not the host, so the host ABI sizes
(`sizeof(int)`, `sizeof(long)`, pointer width) will be wrong for the target.
Two mitigations:

- **Approach A**: Supply a custom `Config` with a hand-crafted `Predefined`
  string (target macros: `__SIZEOF_INT__=2`, `__SIZEOF_POINTER__=2`, etc.) and
  target `IncludePaths` pointing to our own minimal headers. No host compiler
  needed at all.
- **Approach B**: Use the host compiler only to bootstrap `Config`, then
  override `ABI` with a custom one. Less work but ties us to the host toolchain
  on the development machine.

Approach A is best for distribution — the compiler becomes self-contained.

## 3. `#include` and header resolution

`Config.IncludePaths` is searched for `#include "foo.h"`.
`Config.SysIncludePaths` for `#include <foo.h>`.
The special sentinel `"@"` means *the directory of the including file*.

`Config.FS` (an `fs.FS`) can override the filesystem — enabling us to embed a
minimal C standard header library inside the MiniGolf binary using `//go:embed`.

### A MiniGolf-specific C header

We would ship a small `minigolf.h` (or bundle it as a `Source` prepended to
every translation):

```c
/* minigolf.h — MiniGolf C preamble */
typedef unsigned char  byte;
typedef unsigned int   word;
extern void putchar(char ch);
/* ... further MiniGolf builtins ... */
```

Users just write `#include <minigolf.h>` (or we inject it automatically).

## 4. Packaging as a C99 frontend in the MiniGolf compiler

### Option A — Pipeline flag (`-lang=c`)

Add a `-lang=c` (or `-c`) flag to the `minigolf` binary.  When set:
1. Read the `.c` source file.
2. Run `cc_to_golf` translation in-process (no subprocess) → Golf AST in memory.
3. Feed the Golf AST directly into the existing IR builder — **skip writing a
   `.golf` file entirely**.

This is the cleanest approach for the user:
```
minigolf -m=m6809 -lang=c -o out.asm myprogram.c
```

The MiniGolf intermediate representation stays internal.

### Option B — Explicit two-step (keep `.golf` as visible intermediate)

```
cc_to_golf myprogram.c > myprogram.golf
minigolf -m=m6809 -o out.asm myprogram.golf
```

Good for debugging — user can inspect the Golf output. Already working today.

### Option C — Driver script / wrapper

A thin `mgcc` shell script (or Go binary):
```sh
#!/bin/sh
golf=$(mktemp /tmp/XXXX.golf)
cc_to_golf "$1" > "$golf"
minigolf -m="${MGCC_ARCH:-m6809}" -o "$2" "$golf"
rm "$golf"
```

Composable with make/cmake as a drop-in `CC` replacement (limited).

### Recommended approach: A + B

- Ship **Option A** for end users: `minigolf -lang=c -m=m6809 foo.c`
- Keep **Option B** available via `cc_to_golf` for debugging/inspection
- Embed the MiniGolf preamble headers using `//go:embed` so no external files
  needed at runtime

## 5. Implementation plan for Option A (in-process C frontend)

### What changes

#### `cc_v5/cmd/cc_to_golf/cc_to_golf.go` → `cc_v5/translator/translator.go`

- Move `translator` struct and `translateProgram` into a proper package
  (`package translator`) with a public `Translate(src string) (string, error)` API.
- The `main()` function becomes a thin CLI wrapper.

#### `main.go` (MiniGolf compiler entry point)

- Add `-lang` flag (`golf` default, `c` for C99 input).
- When `-lang=c`:
  ```go
  golfSrc, err := translator.TranslateFile(inputFile, cfg)
  // then parse golfSrc and feed to IR builder as normal
  ```

#### `cc_v5/config/` — embedded headers

- Add `//go:embed headers/` to embed `minigolf.h` and possibly a minimal
  `stdint.h`, `stdbool.h`.
- Pass an `fs.FS` to `cc.Config` so `#include <minigolf.h>` resolves from the
  embedded FS without needing a host compiler or system headers.

#### Custom `Config` (no host compiler dependency)

```go
cfg := &cc.Config{
    ABI:      minigolfABI,          // custom 8/16-bit ABI
    Predefined: minigolfPredefined, // embedded macro set
    IncludePaths:    []string{"@"}, // local includes
    SysIncludePaths: []string{},    // served from embedded FS
    FS:       embedFS,
}
```

The `minigolfABI` sets `sizeof(int)=2`, `sizeof(long)=4`,
`sizeof(pointer)=2` to match the M6809 target (or overrideable per `-m` flag).

## 6. Open questions

> [!IMPORTANT]
> **ABI per backend**: M6809 uses 16-bit pointers and 2-byte int. CBE/x86_64
> use native host sizes. Should `-lang=c` automatically pick the right ABI from
> the `-m` flag, or should users set it explicitly?

> [!NOTE]
> **Standard library**: `#include <stdio.h>` etc. pull in POSIX/GNU headers
> that reference many types we don't support. We should document that only
> `<minigolf.h>` is supported for now. The `cc.Config.FS` override means we
> can provide stubs for common headers if desired.

> [!TIP]
> **`NewConfig` caching**: `NewConfig` is documented as expensive (spawns a
> subprocess). Cache it at process start, or pre-generate it and bake it into
> the binary with Approach A's custom Config — no subprocess at all.
