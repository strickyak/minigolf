package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/cbe"
	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/m6809"
	"github.com/strickyak/minigolf/opt"
	"github.com/strickyak/minigolf/parser"
	"github.com/strickyak/minigolf/prelude"
	"github.com/strickyak/minigolf/semantic"
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

	var initFuncsByModule = make(map[string][]string)
	var initCounter int

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
			if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
				if funcStmt.Name.Value == "init" {
					initName := fmt.Sprintf("init_%d", initCounter)
					initCounter++
					funcStmt.Name.Value = initName
					initFuncsByModule[overridePackage] = append(initFuncsByModule[overridePackage], initName)
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

	var moduleNames []string
	for mod := range initFuncsByModule {
		moduleNames = append(moduleNames, mod)
	}
	sort.Strings(moduleNames)

	var allInitCalls []ast.Statement
	for _, mod := range moduleNames {
		for _, initName := range initFuncsByModule[mod] {
			var funcExpr ast.Expression
			if mod == "main" {
				funcExpr = &ast.Identifier{Value: initName}
			} else {
				funcExpr = &ast.SelectorExpression{
					Left:  &ast.Identifier{Value: mod},
					Right: &ast.Identifier{Value: initName},
				}
			}
			callExpr := &ast.CallExpression{
				Function: funcExpr,
			}
			exprStmt := &ast.ExpressionStatement{
				Expression: callExpr,
			}
			allInitCalls = append(allInitCalls, exprStmt)
		}
	}

	if len(allInitCalls) > 0 {
		var mainFunc *ast.FuncStatement
		var mainPackage string
		for _, stmt := range program.Statements {
			if pkgStmt, ok := stmt.(*ast.PackageStatement); ok {
				mainPackage = pkgStmt.Name.Value
			}
			if funcStmt, ok := stmt.(*ast.FuncStatement); ok {
				if mainPackage == "main" && funcStmt.Name.Value == "main" {
					mainFunc = funcStmt
					break
				}
			}
		}
		if mainFunc != nil && mainFunc.Body != nil {
			mainFunc.Body.Statements = append(allInitCalls, mainFunc.Body.Statements...)
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

	noConstfold := flag.Bool("no-constfold", false, "Disable Constant Folding optimization")
	noDbe := flag.Bool("no-dbe", false, "Disable Dead Branch Elimination optimization")
	noDce := flag.Bool("no-dce", false, "Disable Dead Code Elimination optimization")
	noDfe := flag.Bool("no-dfe", false, "Disable Dead Function Elimination optimization")
	noCopyProp := flag.Bool("no-copyprop", false, "Disable Copy Propagation optimization")
	noCse := flag.Bool("no-cse", false, "Disable Common Subexpression Elimination optimization")
	noStrengthRed := flag.Bool("no-strengthred", false, "Disable Strength Reduction optimization")
	noPhisimp := flag.Bool("no-phisimp", false, "Disable Phi Simplification optimization")
	noStackAlloc := flag.Bool("no-stackalloc", false, "Disable Stack Slot Allocation (Slot Sharing)")
	noBranchFold := flag.Bool("no-branchfold", false, "Disable Branch Folding optimization")
	debugOpt := flag.Bool("debug_opt", false, "Enable debug output for optimizations like leaf level")
	checkBoundsFlag := flag.Bool("check-bounds", false, "Enable bounds checking for slices and arrays")
	checkNilFlag := flag.Bool("check-nil", false, "Enable nil pointer checks for pointers, method receivers, and function references")

	var importDirPath repeatedFlag
	flag.Var(&importDirPath, "I", "directory to be searched for imports")

	var defineFlags repeatedFlag
	flag.Var(&defineFlags, "D", "Override constant value: -Dmodule.const=value")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -m <architecture> -o <output_file> <source_files...>\n", os.Args[0])
		flag.PrintDefaults()
	}

	// Parse command-line flags
	flag.Parse()

	// Apply environment variable overrides for optimization flags
	if os.Getenv("NO_CONSTFOLD") != "" {
		*noConstfold = true
	}
	if os.Getenv("NO_DBE") != "" {
		*noDbe = true
	}
	if os.Getenv("NO_DCE") != "" {
		*noDce = true
	}
	if os.Getenv("NO_DFE") != "" {
		*noDfe = true
	}
	if os.Getenv("NO_COPYPROP") != "" {
		*noCopyProp = true
	}
	if os.Getenv("NO_CSE") != "" {
		*noCse = true
	}
	if os.Getenv("NO_STRENGTHRED") != "" {
		*noStrengthRed = true
	}
	if os.Getenv("NO_PHISIMP") != "" {
		*noPhisimp = true
	}
	if os.Getenv("NO_STACKALLOC") != "" {
		*noStackAlloc = true
	}
	if os.Getenv("NO_BRANCHFOLD") != "" {
		*noBranchFold = true
	}

	// Validate required flags
	if *archFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: Target architecture flag (-m) is required.")
		flag.Usage()
		os.Exit(1)
	}

	// Parse -D overrides
	defines := make(map[string]string)
	for _, d := range defineFlags {
		parts := strings.SplitN(d, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Error: Invalid syntax for -D flag '%s'. Expected format: -Dmodule.const=value\n", d)
			os.Exit(1)
		}
		defines[parts[0]] = parts[1]
	}

	// Remaining arguments are source files
	sourceFiles := flag.Args()
	if len(sourceFiles) != 1 {
		fmt.Fprintln(os.Stderr, "Error: Exactly one GOLF source file must be provided.")
		flag.Usage()
		os.Exit(1)
	}
	mainSourceFile := sourceFiles[0]

	if *outFlag != "" {
		logFilename := *outFlag + ".log"
		logFile, err := os.Create(logFilename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Fatal error: could not create log file %s: %v\n", logFilename, err)
			os.Exit(1)
		}
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	log.SetFlags(log.Lshortfile)
	log.SetPrefix("#G ")

	log.Printf("Starting whole-program compilation")
	log.Printf("Target architecture: %s", *archFlag)
	log.Printf("FramePointer: %v", *framePointerFlag)
	log.Printf("Globals-at-Y: %v", *globalsAtYFlag)
	log.Printf("Position-Independent Code: %v", *picFlag)
	log.Printf("Output object file: %s", *outFlag)
	log.Printf("import path: %v", importDirPath)
	log.Printf("Source files: %v", sourceFiles)

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
		log.Printf("Successfully dumped AST to: %s", *outFlag)
		os.Exit(0)
	}

	resolver := semantic.NewResolver(defines)
	resolver.Resolve(program)
	resolveCallback := func(node ast.Node, defPkg string) ast.Node {
		if stmt, ok := node.(ast.Statement); ok {
			return resolver.ResolveGenericInst(stmt, defPkg)
		}
		if expr, ok := node.(ast.Expression); ok {
			return resolver.ResolveGenericInstExpr(expr, defPkg)
		}
		return node
	}

	// 3. Perform semantic analysis & type checking.
	analyzer := semantic.New(resolver)
	analyzer.Analyze(program)
	if len(analyzer.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Semantic errors:")
		for _, e := range analyzer.Errors() {
			fmt.Fprintf(os.Stderr, "\t%s\n", e)
		}
		os.Exit(1)
	}

	analyzer.TrimDeadFunctions(program)
	program.MarkTrunkFunctions(analyzer.ResolveFunc)

	if *debugOpt {
		for _, stmt := range program.Statements {
			if fs, ok := stmt.(*ast.FuncStatement); ok {
				log.Printf("TrunkLevel: %s is level %d", fs.Name.Value, fs.TrunkLevel)
			}
		}
	}

	if val, ok := analyzer.Pragmas["CHECK_BOUNDS"]; ok {
		*checkBoundsFlag = (val == "1" || val == "true")
	}
	if val, ok := analyzer.Pragmas["CHECK_NIL"]; ok {
		*checkNilFlag = (val == "1" || val == "true")
	}

	// Flag -m=ir : emit SSA IR and exit cleanly
	if *archFlag == "IR" {
		builder := ir.NewBuilder(resolveCallback, 8)
		builder.CheckBounds = *checkBoundsFlag
		builder.CheckNil = *checkNilFlag
		irProg := builder.Build(program)

		optConfig := opt.Config{
			EnableConstFold:   !*noConstfold,
			EnableDBE:         !*noDbe,
			EnableDCE:         !*noDce,
			EnableCopyProp:    !*noCopyProp,
			EnableCSE:         !*noCse,
			EnableStrengthRed: !*noStrengthRed,
			EnablePhiSimp:     !*noPhisimp,
			EnableStackAlloc:  !*noStackAlloc,
			EnableBranchFold:  !*noBranchFold,
			EnableDFE:         !*noDfe,
			EnableDebugOpt:    *debugOpt,
		}
		builder.AnnotateLeafLevels(*debugOpt)
		opt.OptimizeProgram(irProg, optConfig)
		builder.AnnotateLeafLevels(*debugOpt)
		irCode := ir.PrintProgram(irProg)

		header := fmt.Sprintf("; Starting whole-program compilation\n; Target architecture: %s\n; Output object file: %s\n; Source files: %v\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + irCode

		err := writeOutput(*outFlag, finalOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing IR output: %v\n", err)
			os.Exit(1)
		}
		log.Printf("Successfully compiled to IR: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=cbe : Generate C from IR and exit cleanly
	if *archFlag == "CBE" {
		builder := ir.NewBuilder(resolveCallback, 8)
		builder.CheckBounds = *checkBoundsFlag
		builder.CheckNil = *checkNilFlag
		irProg := builder.Build(program)

		optConfig := opt.Config{
			EnableConstFold:   !*noConstfold,
			EnableDBE:         !*noDbe,
			EnableDCE:         !*noDce,
			EnableCopyProp:    !*noCopyProp,
			EnableCSE:         !*noCse,
			EnableStrengthRed: !*noStrengthRed,
			EnablePhiSimp:     !*noPhisimp,
			EnableStackAlloc:  !*noStackAlloc,
			EnableBranchFold:  !*noBranchFold,
			EnableDFE:         !*noDfe,
			EnableDebugOpt:    *debugOpt,
		}
		builder.AnnotateLeafLevels(*debugOpt)
		opt.OptimizeProgram(irProg, optConfig)
		builder.AnnotateLeafLevels(*debugOpt)

		backend := cbe.New()
		cCode := backend.Generate(irProg)

		header := fmt.Sprintf("/*\n * Starting whole-program compilation (CBE Backend)\n * Target architecture: %s\n * Output object file: %s\n * Source files: %v\n */\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + cCode

		err := writeOutput(*outFlag, finalOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing CBE output: %v\n", err)
			os.Exit(1)
		}
		log.Printf("Successfully compiled via CBE to: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=x86_64 : Generate X86_64 assembly from IR and exit cleanly
	if *archFlag == "X86_64" || *archFlag == "X86-64" || *archFlag == "X" {
		builder := ir.NewBuilder(resolveCallback, 8)
		builder.CheckBounds = *checkBoundsFlag
		builder.CheckNil = *checkNilFlag
		irProg := builder.Build(program)

		optConfig := opt.Config{
			EnableConstFold:   !*noConstfold,
			EnableDBE:         !*noDbe,
			EnableDCE:         !*noDce,
			EnableCopyProp:    !*noCopyProp,
			EnableCSE:         !*noCse,
			EnableStrengthRed: !*noStrengthRed,
			EnablePhiSimp:     !*noPhisimp,
			EnableStackAlloc:  !*noStackAlloc,
			EnableBranchFold:  !*noBranchFold,
			EnableDFE:         !*noDfe,
			EnableDebugOpt:    *debugOpt,
		}
		builder.AnnotateLeafLevels(*debugOpt)
		opt.OptimizeProgram(irProg, optConfig)
		builder.AnnotateLeafLevels(*debugOpt)

		backend := x86_64.New()
		asmCode := backend.Generate(irProg)

		header := fmt.Sprintf("/*\n * Starting whole-program compilation (X86_64 Backend)\n * Target architecture: %s\n * Output object file: %s\n * Source files: %v\n */\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + asmCode

		err := writeOutput(*outFlag, finalOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing X86_64 output: %v\n", err)
			os.Exit(1)
		}
		log.Printf("Successfully compiled via X86_64 to: %s", *outFlag)
		os.Exit(0)
	}

	// Flag -m=6809 : Generate M6809 assembly from IR and exit cleanly
	if *archFlag == "6809" || *archFlag == "M6809" || *archFlag == "M" {
		builder := ir.NewBuilder(resolveCallback, 2)
		builder.CheckBounds = *checkBoundsFlag
		builder.CheckNil = *checkNilFlag
		irProg := builder.Build(program)

		optConfig := opt.Config{
			EnableConstFold:  !*noConstfold,
			EnableDBE:        !*noDbe,
			EnableDCE:        !*noDce,
			EnableCopyProp:   !*noCopyProp,
			EnablePhiSimp:    !*noPhisimp,
			EnableStackAlloc: !*noStackAlloc,
			EnableBranchFold: !*noBranchFold,
			EnableDFE:        !*noDfe,
			EnableDebugOpt:   *debugOpt,
		}
		builder.AnnotateLeafLevels(*debugOpt)
		opt.OptimizeProgram(irProg, optConfig)
		builder.AnnotateLeafLevels(*debugOpt)

		backend := m6809.New(*framePointerFlag, *globalsAtYFlag, *picFlag)
		asmCode := backend.Generate(irProg)

		header := fmt.Sprintf(";\n; Starting whole-program compilation (Motorola 6809 Backend)\n; Target architecture: %s\n; Output object file: %s\n; Source files: %v\n;\n\n", *archFlag, *outFlag, sourceFiles)
		finalOutput := header + asmCode

		err := writeOutput(*outFlag, finalOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing 6809 output: %v\n", err)
			os.Exit(1)
		}
		log.Printf("Successfully compiled via 6809 to: %s", *outFlag)
		os.Exit(0)
	}

	// For other values of -m, panic for now
	panic("Architecture " + *archFlag + " not yet implemented.")
}

func writeOutput(outFlag string, finalOutput string) error {
	if outFlag == "" {
		_, err := os.Stdout.WriteString(finalOutput)
		return err
	}
	return os.WriteFile(outFlag, []byte(finalOutput), 0644)
}
