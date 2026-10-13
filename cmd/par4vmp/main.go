package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/strickyak/minigolf/par4"
)

func main() {
	verboseFlag := flag.Bool("v", false, "Verbose execution trace")
	traceFlag := flag.Bool("trace", false, "Trace instructions")
	maxInstFlag := flag.Int("max-inst", 0, "Maximum instructions to execute (0 = unlimited)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: par4vmp [options] <program.p4p>\n\n")
		fmt.Fprintf(os.Stderr, "Par4 Virtual Machine - POSIX / PC Execution Engine\n\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	filename := flag.Arg(0)
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file %q: %v\n", filename, err)
		os.Exit(1)
	}

	vm, err := par4.NewVM(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading binary %q: %v\n", filename, err)
		os.Exit(1)
	}

	vm.Verbose = *verboseFlag || *traceFlag
	vm.MaxInstructions = *maxInstFlag

	exitCode, runErr := vm.Run()
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "\n%v\n", runErr)
		os.Exit(1)
	}

	os.Exit(exitCode)
}
