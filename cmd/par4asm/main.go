package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/strickyak/minigolf/par4"
)

func main() {
	outFlag := flag.String("o", "", "Output binary path (.p4p)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: par4asm [options] <input.p4a>\n\n")
		fmt.Fprintf(os.Stderr, "Par4 Bytecode Assembler (*.p4a -> *.p4p)\n\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputFile := flag.Arg(0)
	srcBytes, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %q: %v\n", inputFile, err)
		os.Exit(1)
	}

	bin, err := par4.Assemble(string(srcBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Assembly Error: %v\n", err)
		os.Exit(1)
	}

	outFile := *outFlag
	if outFile == "" {
		if strings.HasSuffix(inputFile, ".p4a") {
			outFile = strings.TrimSuffix(inputFile, ".p4a") + ".p4p"
		} else if strings.HasSuffix(inputFile, ".p3a") {
			outFile = strings.TrimSuffix(inputFile, ".p3a") + ".p4p"
		} else {
			outFile = inputFile + ".p4p"
		}
	}

	if err := os.WriteFile(outFile, bin, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file %q: %v\n", outFile, err)
		os.Exit(1)
	}

	fmt.Printf("Assembled %q -> %q (%d bytes).\n", inputFile, outFile, len(bin))
}
