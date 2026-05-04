package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"minigo/ast"
	"minigo/lexer"
	"minigo/parser"
	"minigo/semantic"
	"minigo/transpiler"
)

func main() {
	// Define command-line flags
	archFlag := flag.String("m", "", "Target architecture (e.g., 6809, 6309, x86_64, z80, 6502)")
	outFlag := flag.String("o", "", "Output object file name")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -m <architecture> -o <output_file> <source_files...>\n", os.Args[0])
		flag.PrintDefaults()
	}

	// Parse command-line flags
	flag.Parse()

	// Validate required flags
	if *archFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: Target architecture flag (-m) is required.")
		flag.Usage()
		os.Exit(1)
	}
	if *outFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: Output object file flag (-o) is required.")
		flag.Usage()
		os.Exit(1)
	}

	// Remaining arguments are source files
	sourceFiles := flag.Args()
	if len(sourceFiles) == 0 {
		fmt.Fprintln(os.Stderr, "Error: At least one source file must be provided.")
		flag.Usage()
		os.Exit(1)
	}

	// Create a log file based on the output filename for debugging
	logFilename := *outFlag + ".log"
	logFile, err := os.Create(logFilename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: could not create log file %s: %v\n", logFilename, err)
		os.Exit(1)
	}
	defer logFile.Close()

	// Configure a dedicated logger
	logger := log.New(logFile, "minigo: ", log.Ltime|log.Lshortfile)

	logger.Printf("Starting whole-program compilation")
	logger.Printf("Target architecture: %s", *archFlag)
	logger.Printf("Output object file: %s", *outFlag)
	logger.Printf("Source files: %v", sourceFiles)

	// =========================================================================
	// Compilation Pipeline
	// =========================================================================
	// 1 & 2. Parse all source files into a single flat namespace AST
	var program *ast.Program
	for _, filename := range sourceFiles {
		content, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file %s: %v\n", filename, err)
			os.Exit(1)
		}
		
		tokens := lexer.Lex(string(content))
		p := parser.New(tokens)
		fileProgram := p.ParseProgram()
		
		if len(p.Errors()) > 0 {
			fmt.Fprintf(os.Stderr, "Parser errors in %s:\n", filename)
			for _, e := range p.Errors() {
				fmt.Fprintf(os.Stderr, "\t%s\n", e)
			}
			os.Exit(1)
		}
		
		if program == nil {
			program = fileProgram
		} else {
			program.Statements = append(program.Statements, fileProgram.Statements...)
		}
	}

	// 3. Perform semantic analysis & type checking.
	analyzer := semantic.New()
	analyzer.Analyze(program)
	if len(analyzer.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Semantic errors:")
		for _, e := range analyzer.Errors() {
			fmt.Fprintf(os.Stderr, "\t%s\n", e)
		}
		os.Exit(1)
	}
	
	// Flag -m=C : transpile AST to C and exit cleanly
	if *archFlag == "C" || *archFlag == "c" || *archFlag == "c99" || *archFlag == "C99" {
		tr := transpiler.New()
		cCode := tr.Transpile(program)
		
		header := fmt.Sprintf("/*\n * Starting whole-program compilation\n * Target architecture: %s\n * Output object file: %s\n * Source files: %v\n */\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + cCode
		
		err = os.WriteFile(*outFlag, []byte(finalOutput), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing C output: %v\n", err)
			os.Exit(1)
		}
		logger.Printf("Successfully compiled to C99: %s", *outFlag)
		os.Exit(0)
	}

	// For other values of -m, panic for now
	panic("Architecture " + *archFlag + " not yet implemented. Use -m=C for the transpiler.")
}
