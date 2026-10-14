package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/strickyak/minigolf/par4"
)

func main() {
	outFlag := flag.String("o", "", "Output disassembly path (.p4s)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: par4dis [options] <input.p4p>\n\n")
		fmt.Fprintf(os.Stderr, "Par4 Bytecode Disassembler (*.p4p -> *.p4s)\n\nOptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputFile := flag.Arg(0)
	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %q: %v\n", inputFile, err)
		os.Exit(1)
	}

	disText, err := par4.Disassemble(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Disassembly Error: %v\n", err)
		os.Exit(1)
	}

	outFile := *outFlag
	if outFile != "" {
		if err := os.WriteFile(outFile, []byte(disText), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output file %q: %v\n", outFile, err)
			os.Exit(1)
		}
		fmt.Printf("Disassembled %q -> %q\n", inputFile, outFile)
	} else {
		// Output to stdout
		fmt.Print(disText)
	}
	_ = strings.Clone
}
