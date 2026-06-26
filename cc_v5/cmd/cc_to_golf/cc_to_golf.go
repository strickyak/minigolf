// cc_to_golf — standalone C-to-MiniGolf translation tool.
//
// This is a thin shim around the cc_v5/translator package, which is also
// built into the minigolf compiler itself (see -m=cc_to_golf and .c source
// detection in main.go).
//
// Usage:
//
//	cc_to_golf [-k] [-I=dir] [-D=NAME=value] <file.c>
//
// Flags follow Go flag conventions (=  or space separator, NOT -Idir/-Dname).
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/strickyak/minigolf/ctranslator"
)

var keepGoing = flag.Bool("k", false,
	"keep going after unsupported constructs (emit /* comment */ instead of panic)")

func main() {
	var includePaths repeatedFlag
	flag.Var(&includePaths, "I", "directory to search for #include files (repeatable; last dir is for <...>)")

	var defineFlags repeatedFlag
	flag.Var(&defineFlags, "D", "C preprocessor definition: -D=NAME or -D=NAME=value (repeatable)")

	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: cc_to_golf [-k] [-I=dir ...] [-D=NAME[=val] ...] <file.c>")
		os.Exit(1)
	}

	defines := make(map[string]string)
	for _, d := range defineFlags {
		parts := strings.SplitN(d, "=", 2)
		if len(parts) == 2 {
			defines[parts[0]] = parts[1]
		} else {
			defines[parts[0]] = ""
		}
	}

	golfSrc, warn := ctranslator.TranslateFile(args[0], ctranslator.Options{
		IncludePaths: []string(includePaths),
		Defines:      defines,
		KeepGoing:    *keepGoing,
	})
	if warn != nil {
		fmt.Fprintln(os.Stderr, warn)
	}
	if golfSrc == "" {
		os.Exit(1)
	}
	fmt.Print(golfSrc)
}

// repeatedFlag collects multiple -flag=value occurrences.
type repeatedFlag []string

func (f *repeatedFlag) String() string  { return strings.Join(*f, ",") }
func (f *repeatedFlag) Set(v string) error { *f = append(*f, v); return nil }
