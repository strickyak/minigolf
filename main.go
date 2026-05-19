package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/cbe"
	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/m6809"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/prelude"
	"github.com/strickyak/minigolf/semantic"
	"github.com/strickyak/minigolf/transpiler"
	"github.com/strickyak/minigolf/x86_64"
)

// Define a custom type that is a slice of strings
type repeatedFlag []string

// String is the method to format the flag's value, part of the flag.Value interface.
// The output of this method is used in diagnostics.
func (f *repeatedFlag) String() string {
	return strings.Join(*f, ", ")
}

// Set is called by the flag package each time the flag is seen on the command line.
func (f *repeatedFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

var MatchValidImport = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`).FindStringSubmatch

func ReadFileFromPath(base string, path []string) (content []byte, err error) {
    log.Printf("RFFP: want %q with path %v", base, path)
	for _, d := range path {
        filename := filepath.Join(d, base)
		content, err = os.ReadFile(filename)
        log.Printf("RFFP: Looking for %q ... %v", filename, err)
		if err == nil {
			return
		}
	}

	// If "prelude.golf" is not found in the path, we use the version included in this compiler.
	if base == "prelude.golf" {
		content = []byte(prelude.Source)
		err = nil
        return
	}

	return nil, fmt.Errorf("Cannot find filename %q in path %v", base, path)
}

func ParseSourceFiles(mainSourceFile string, importDirPath repeatedFlag) *ast.Program {
	var program *ast.Program
	imported := make(map[string]bool)

	// Build new path with initial directory on the front.
	mainDirname := filepath.Dir(mainSourceFile)
	path := []string{mainDirname}
	for _, d := range importDirPath {
		path = append(path, d)
	}

	slurp := func(filename string, overridePackage string, path []string) {
		var content []byte
		var err error
		if path == nil {
			content, err = os.ReadFile(filename)
		} else {
			content, err = ReadFileFromPath(filename, path)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file %s: %v\n", filename, err)
			os.Exit(1)
		}

		tokens := lexer.Lex(string(content), filename)
		p := parser.New(tokens)

		fileProgram := p.ParseProgram(overridePackage)

		if len(p.Errors()) > 0 {
			fmt.Fprintf(os.Stderr, "Parser errors in %s:\n", filename)
			for _, e := range p.Errors() {
				fmt.Fprintf(os.Stderr, "\t%s\n", e)
			}
			os.Exit(1)
		}

		for _, stmt := range fileProgram.Statements {
			if imp, ok := stmt.(*ast.ImportStatement); ok {
				s := imp.Path.Value
				if m := MatchValidImport(s); m == nil {
					log.Panicf("Bad import syntax (should match `^[A-Za-z][A-Za-z0-9_]*$`): %q", s)
				}
				if _, ok := imported[s]; !ok {
					imported[s] = false
				}
			}
		}

		if program == nil {
			program = fileProgram
		} else {
			program.Statements = append(program.Statements, fileProgram.Statements...)
		}
	}

	imported["prelude"] = true
	slurp("prelude.golf", "prelude", path)

	imported["main"] = true
	slurp(mainSourceFile, "main", nil)

MORE:
	for key, done := range imported {
		if !done {
			imported[key] = true
			slurp(key+".golf", key, path)
			// We changed the iterated object, so restart the iteration.
			// When everything is done, we fall through and return the program.
			goto MORE
		}
	}
	return program
}

func main() {
	// Define command-line flags
	archFlag := flag.String("m", "", "Target architecture (e.g., 6809, 6309, x86_64, z80, 6502)")
	outFlag := flag.String("o", "", "Output object file name")
	framePointerFlag := flag.Bool("frame-pointer", false, "Use a dedicated hardware frame pointer (U register) instead of computing offsets from S")
	globalsAtYFlag := flag.Bool("globals-at-y", false, "Reserve Y register as a pointer to the global data section (uses contiguous offset addressing)")
	picFlag := flag.Bool("pic", false, "Generate position-independent code (PIC) using relative branches and localized PCR data segments")

	var importDirPath repeatedFlag
	flag.Var(&importDirPath, "I", "directory to be searched for imports")

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

	// Remaining arguments are source files
	sourceFiles := flag.Args()
	if len(sourceFiles) != 1 {
		fmt.Fprintln(os.Stderr, "Error: Exactly one GOLF source file must be provided.")
		flag.Usage()
		os.Exit(1)
	}
	mainSourceFile := sourceFiles[0]

	var logger *log.Logger
	if *outFlag != "" {
		logFilename := *outFlag + ".log"
		logFile, err := os.Create(logFilename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Fatal error: could not create log file %s: %v\n", logFilename, err)
			os.Exit(1)
		}
		defer logFile.Close()
		logger = log.New(logFile, "minigo: ", log.Lshortfile)
	} else {
		logger = log.New(os.Stderr, "minigo: ", log.Lshortfile)
	}

	logger.Printf("Starting whole-program compilation")
	logger.Printf("Target architecture: %s", *archFlag)
	logger.Printf("Output object file: %s", *outFlag)
	logger.Printf("import path: %v", importDirPath)
	logger.Printf("Source files: %v", sourceFiles)

	// =========================================================================
	// Compilation Pipeline
	// =========================================================================
	// 1 & 2. Parse all source files into a single flat namespace AST
	program := ParseSourceFiles(mainSourceFile, importDirPath)

	*archFlag = strings.ToUpper(*archFlag)

	// Flag -m=ast : print the AST and exit cleanly
	if *archFlag == "AST" {
		astOutput := ast.Print(program)
		header := fmt.Sprintf("; Starting whole-program compilation\n; Target architecture: %s\n; Output object file: %s\n; Source files: %v\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + astOutput + "\n"

		err := writeOutput(*outFlag, finalOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing AST output: %v\n", err)
			os.Exit(1)
		}
		logger.Printf("Successfully dumped AST to: %s", *outFlag)
		os.Exit(0)
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

	// Flag -m=ir : emit SSA IR and exit cleanly
	if *archFlag == "IR" {
		builder := ir.NewBuilder()
		irProg := builder.Build(program)
		irCode := ir.PrintProgram(irProg)

		header := fmt.Sprintf("; Starting whole-program compilation\n; Target architecture: %s\n; Output object file: %s\n; Source files: %v\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + irCode

		err := writeOutput(*outFlag, finalOutput)
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

		err := writeOutput(*outFlag, finalOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing CBE output: %v\n", err)
			os.Exit(1)
		}
		logger.Printf("Successfully compiled via CBE to: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=x86_64 : Generate X86_64 assembly from IR and exit cleanly
	if *archFlag == "X86_64" || *archFlag == "X86-64" || *archFlag == "X" {
		builder := ir.NewBuilder()
		irProg := builder.Build(program)

		backend := x86_64.New()
		asmCode := backend.Generate(irProg)

		header := fmt.Sprintf("/*\n * Starting whole-program compilation (X86_64 Backend)\n * Target architecture: %s\n * Output object file: %s\n * Source files: %v\n */\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + asmCode

		err := writeOutput(*outFlag, finalOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing X86_64 output: %v\n", err)
			os.Exit(1)
		}
		logger.Printf("Successfully compiled via X86_64 to: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=6809 : Generate M6809 assembly from IR and exit cleanly
	if *archFlag == "6809" || *archFlag == "M6809" || *archFlag == "M" {
		builder := ir.NewBuilder()
		irProg := builder.Build(program)

		backend := m6809.New(*framePointerFlag, *globalsAtYFlag, *picFlag)
		asmCode := backend.Generate(irProg)

		header := fmt.Sprintf(";\n; Starting whole-program compilation (Motorola 6809 Backend)\n; Target architecture: %s\n; Output object file: %s\n; Source files: %v\n;\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + asmCode

		err := writeOutput(*outFlag, finalOutput)
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

		err := writeOutput(*outFlag, finalOutput)
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

func writeOutput(outFlag string, finalOutput string) error {
	if outFlag == "" {
		_, err := os.Stdout.WriteString(finalOutput)
		return err
	}
	return os.WriteFile(outFlag, []byte(finalOutput), 0644)
}
