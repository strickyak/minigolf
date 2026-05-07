package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"minigo/ast"
	"minigo/cbe"
	"minigo/ir"
	"minigo/lexer"
	"minigo/parser"
	"minigo/semantic"
	"minigo/transpiler"
	"minigo/x86_64"
	"minigo/m6809"
	"strings"
)

func main() {
	// Define command-line flags
	archFlag := flag.String("m", "", "Target architecture (e.g., 6809, 6309, x86_64, z80, 6502)")
	outFlag := flag.String("o", "", "Output object file name")
	framePointerFlag := flag.Bool("frame-pointer", false, "Use a dedicated hardware frame pointer (U register) instead of computing offsets from S")
	globalsAtYFlag := flag.Bool("globals-at-y", false, "Reserve Y register as a pointer to the global data section (uses contiguous offset addressing)")
	picFlag := flag.Bool("pic", false, "Generate position-independent code (PIC) using relative branches and localized PCR data segments")

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
		
		tokens := lexer.Lex(string(content), filename)
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
	
	*archFlag = strings.ToUpper(*archFlag)

	// Flag -m=ir : emit SSA IR and exit cleanly
	if *archFlag == "IR" {
		builder := ir.NewBuilder()
		irProg := builder.Build(program)
		irCode := ir.PrintProgram(irProg)
		
		header := fmt.Sprintf("; Starting whole-program compilation\n; Target architecture: %s\n; Output object file: %s\n; Source files: %v\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + irCode
		
		err = os.WriteFile(*outFlag, []byte(finalOutput), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing IR output: %v\n", err)
			os.Exit(1)
		}
		logger.Printf("Successfully compiled to IR: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=cbe : Generate C from IR and exit cleanly
	if *archFlag == "CBE" {
		builder := ir.NewBuilder()
		irProg := builder.Build(program)
		
		backend := cbe.New()
		cCode := backend.Generate(irProg)
		
		header := fmt.Sprintf("/*\n * Starting whole-program compilation (CBE Backend)\n * Target architecture: %s\n * Output object file: %s\n * Source files: %v\n */\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + cCode
		
		err = os.WriteFile(*outFlag, []byte(finalOutput), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing CBE output: %v\n", err)
			os.Exit(1)
		}
		logger.Printf("Successfully compiled via CBE to: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=x86_64 : Generate X86_64 assembly from IR and exit cleanly
	if *archFlag == "X86_64" || *archFlag == "X86-64" {
		builder := ir.NewBuilder()
		irProg := builder.Build(program)
		
		backend := x86_64.New()
		asmCode := backend.Generate(irProg)
		
		header := fmt.Sprintf("/*\n * Starting whole-program compilation (X86_64 Backend)\n * Target architecture: %s\n * Output object file: %s\n * Source files: %v\n */\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + asmCode
		
		err = os.WriteFile(*outFlag, []byte(finalOutput), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing X86_64 output: %v\n", err)
			os.Exit(1)
		}
		logger.Printf("Successfully compiled via X86_64 to: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=6809 : Generate M6809 assembly from IR and exit cleanly
	if *archFlag == "6809" || *archFlag == "M6809" {
		builder := ir.NewBuilder()
		irProg := builder.Build(program)
		
		backend := m6809.New(*framePointerFlag, *globalsAtYFlag, *picFlag)
		asmCode := backend.Generate(irProg)
		
		header := fmt.Sprintf(";\n; Starting whole-program compilation (Motorola 6809 Backend)\n; Target architecture: %s\n; Output object file: %s\n; Source files: %v\n;\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + asmCode
		
		err = os.WriteFile(*outFlag, []byte(finalOutput), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing 6809 output: %v\n", err)
			os.Exit(1)
		}
		logger.Printf("Successfully compiled via 6809 to: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=C : transpile AST to C and exit cleanly
	if *archFlag == "C" || *archFlag == "C99" {
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
