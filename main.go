package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/strickyak/minigolf/ast"
	"github.com/strickyak/minigolf/cbe"
	"github.com/strickyak/minigolf/ctranslator"
	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/lexer"
	"github.com/strickyak/minigolf/m6809"
	"github.com/strickyak/minigolf/opt"
	"github.com/strickyak/minigolf/parser"
	// "github.com/strickyak/minigolf/prelude"
	"github.com/strickyak/minigolf/semantic"
	"github.com/strickyak/minigolf/x86_64"

	cclib "modernc.org/cc/v5"
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

	//disabled// // If "prelude.golf" is not found in the path, we use the version included in this compiler.
	//disabled// if base == "prelude.golf" {
	//disabled// 	content = []byte(prelude.Source)
	//disabled// 	err = nil
	//disabled// 	return
	//disabled// }

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

	// =========================================================================
	// -D flag routing
	// =========================================================================
	//
	// A -D flag whose name contains a '.' is a MiniGolf constant override
	// (e.g. -D=mymodule.CONST=42). A -D flag whose name has no '.' is a C
	// preprocessor #define (e.g. -D=DEBUG=1), forwarded to ctranslator when
	// the source is a .c file.
	//
	// NOTE: Go flag convention requires -D=NAME=value or -D NAME=value.
	// The C-compiler style -DNAME (no separator) is NOT supported.
	golfDefines := make(map[string]string)
	cDefines := make(map[string]string)
	for _, d := range defineFlags {
		parts := strings.SplitN(d, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Error: -D=%s has no '=VALUE' part. Use -D=NAME=value\n", d)
			os.Exit(1)
		}
		if strings.Contains(parts[0], ".") {
			golfDefines[parts[0]] = parts[1] // MiniGolf constant
		} else {
			cDefines[parts[0]] = parts[1] // C preprocessor macro
		}
	}

	// Remaining arguments are source files.
	sourceFiles := flag.Args()
	if len(sourceFiles) != 1 {
		fmt.Fprintln(os.Stderr, "Error: Exactly one source file must be provided.")
		flag.Usage()
		os.Exit(1)
	}
	mainSourceFile := sourceFiles[0]

	// =========================================================================
	// -m=NEWCONFIG: spawn the host C compiler to harvest its predefined macros
	// and include paths, then print the result to stdout or -o.
	//
	// Use this once on a new host/target to capture the configuration, then
	// bake it into the compiler as a named built-in config:
	//
	//  1. Run:  minigolf -m=newconfig -o myhost.cfg
	//  2. The output contains two sections:
	//       === PREDEFINED MACROS ===      (content for cc.Config.Predefined)
	//       === INCLUDE PATHS ===          (one path per line)
	//       === SYS INCLUDE PATHS ===      (one path per line)
	//  3. In ctranslator/configs.go (create if absent), add a named entry:
	//       var builtinConfigs = map[string]builtinConfig{
	//           "linux-x86_64": { Predefined: "...", IncludePaths: [...], SysIncludePaths: [...] },
	//       }
	//  4. In ctranslator/translator.go, make TranslateFile check
	//     opts.ConfigName against builtinConfigs before calling cc.NewConfig.
	//     This allows cross-compilation without a host C compiler.
	// =========================================================================
	if strings.ToUpper(*archFlag) == "NEWCONFIG" {
		cfg, err := cclib.NewConfig(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			fmt.Fprintf(os.Stderr, "NewConfig failed: %v\n", err)
			os.Exit(1)
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "=== PREDEFINED MACROS ===\n%s\n", cfg.Predefined)
		fmt.Fprintf(&sb, "=== INCLUDE PATHS ===\n%s\n", strings.Join(cfg.IncludePaths, "\n"))
		fmt.Fprintf(&sb, "=== SYS INCLUDE PATHS ===\n%s\n", strings.Join(cfg.SysIncludePaths, "\n"))
		if err := writeOutput(*outFlag, sb.String()); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing NewConfig output: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// =========================================================================
	// C source detection: if the source file ends in .c, run the C→Golf
	// translation phase first.  The intermediate .golf file is written to
	// <outFlag>.tmp.golf (kept for inspection), or to a temp file if -o is
	// not given (Golf source also printed to stderr for debugging).
	//
	// -m=CC_TO_GOLF stops after this phase.
	// =========================================================================
	if strings.HasSuffix(mainSourceFile, ".c") {
		golfSrc, warn := ctranslator.TranslateFile(mainSourceFile, ctranslator.Options{
			IncludePaths: []string(importDirPath),
			Defines:      cDefines,
		})
		if warn != nil {
			fmt.Fprintln(os.Stderr, warn)
		}
		if golfSrc == "" {
			fmt.Fprintln(os.Stderr, "Error: C translation produced empty output.")
			os.Exit(1)
		}

		// Determine where to write the .tmp.golf intermediate.
		var tmpGolfPath string
		if *outFlag != "" {
			tmpGolfPath = *outFlag + ".tmp.golf"
			if err := os.WriteFile(tmpGolfPath, []byte(golfSrc), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", tmpGolfPath, err)
				os.Exit(1)
			}
		} else {
			// No -o: print Golf source to stderr; write a temp file for the parser.
			fmt.Fprint(os.Stderr, golfSrc)
			f, err := os.CreateTemp("", "minigolf-*.tmp.golf")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cannot create temp file: %v\n", err)
				os.Exit(1)
			}
			f.WriteString(golfSrc)
			f.Close()
			tmpGolfPath = f.Name()
			// Keep it — temp files in the system temp dir are cleaned up on
			// reboot; removing it here would prevent error inspection.
		}
		mainSourceFile = tmpGolfPath

		// -m=CC_TO_GOLF: translation is the only goal, stop here.
		if strings.ToUpper(*archFlag) == "CC_TO_GOLF" {
			if *outFlag == "" {
				// Already printed to stderr above.
			} else {
				fmt.Printf("Wrote %s\n", tmpGolfPath)
			}
			os.Exit(0)
		}
	}

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

	resolver := semantic.NewResolver(golfDefines)
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
	// Tell the analyzer which prelude functions are implicitly called by the IR
	// builder for certain operators.  This prevents TrimDeadFunctions from
	// removing them before the IR builder gets a chance to reference them.
	for _, name := range opt.MagicFuncNames {
		analyzer.AddMagicFunc(name)
	}
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
				log.Printf("TrunkLevel: %s is level %d, Popularity: %d", fs.Name.Value, fs.TrunkLevel, fs.Popularity)
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
		opt.MarkMagicFunctions(irProg)

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
		opt.MarkMagicFunctions(irProg)

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
		opt.MarkMagicFunctions(irProg)

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
		opt.MarkMagicFunctions(irProg)

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
