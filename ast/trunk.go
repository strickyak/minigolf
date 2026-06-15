package ast

import "reflect"

type trunkInfo struct {
	funcStmt  *FuncStatement
	callsites []*FuncStatement
	loopCall  bool
	dynamic   bool
}

// MarkTrunkFunctions analyzes the call graph to determine the TrunkLevel of each function.
// A trunk function is one that is guaranteed to execute at most once.
// Level 1: main.main
// Level N: called from exactly one call site, which is in a Level N-1 function, and not in a loop.
func (p *Program) MarkTrunkFunctions(resolver func(Expression) *FuncStatement) {
	infoMap := make(map[*FuncStatement]*trunkInfo)

	// Initialize tracking struct for all functions
	for _, stmt := range p.Statements {
		if fs, ok := stmt.(*FuncStatement); ok {
			infoMap[fs] = &trunkInfo{funcStmt: fs}
		}
	}

	// Traverse the AST to populate call sites and dynamic references
	for _, stmt := range p.Statements {
		if fs, ok := stmt.(*FuncStatement); ok {
			walkTrunk(fs.Body, fs, 0, false, infoMap, resolver)
		}
	}

	// Find Level 1 trunk function (main)
	var mainFunc *FuncStatement
	for _, stmt := range p.Statements {
		if pkg, ok := stmt.(*PackageStatement); ok && pkg.Name.Value == "main" {
			// Found main package, look for main function
		}
		if fs, ok := stmt.(*FuncStatement); ok {
			// We identify main by its name.
			// Wait, we need to be sure it's the main.main, but if package is main and func is main.
			if fs.Name.Value == "main" {
				mainFunc = fs
			}
		}
	}

	if mainFunc != nil {
		mainFunc.TrunkLevel = 1
	}

	// Propagate trunk levels
	changed := true
	for changed {
		changed = false
		for _, info := range infoMap {
			if info.dynamic || info.loopCall || len(info.callsites) != 1 {
				continue
			}
			caller := info.callsites[0]
			if caller.TrunkLevel > 0 {
				newLevel := caller.TrunkLevel + 1
				if info.funcStmt.TrunkLevel == 0 || info.funcStmt.TrunkLevel > newLevel {
					info.funcStmt.TrunkLevel = newLevel
					changed = true
				}
			}
		}
	}
}

func walkTrunk(node Node, currentFunc *FuncStatement, loopDepth int, inCallFunc bool, infoMap map[*FuncStatement]*trunkInfo, resolver func(Expression) *FuncStatement) {
	if node == nil {
		return
	}
	if reflect.ValueOf(node).Kind() == reflect.Ptr && reflect.ValueOf(node).IsNil() {
		return
	}

	switch n := node.(type) {
	case *Program:
		for _, s := range n.Statements {
			walkTrunk(s, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *FuncStatement:
		// Do not walk nested functions if they existed (minigolf doesn't have them but just in case)
		walkTrunk(n.Body, currentFunc, loopDepth, false, infoMap, resolver)
	case *BlockStatement:
		for _, s := range n.Statements {
			walkTrunk(s, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *IfStatement:
		walkTrunk(n.Condition, currentFunc, loopDepth, false, infoMap, resolver)
		walkTrunk(n.Consequence, currentFunc, loopDepth, false, infoMap, resolver)
		if n.Alternative != nil {
			walkTrunk(n.Alternative, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *ForStatement:
		walkTrunk(n.Condition, currentFunc, loopDepth+1, false, infoMap, resolver)
		walkTrunk(n.Body, currentFunc, loopDepth+1, false, infoMap, resolver)
	case *For3Statement:
		walkTrunk(n.Init, currentFunc, loopDepth, false, infoMap, resolver)
		walkTrunk(n.Condition, currentFunc, loopDepth+1, false, infoMap, resolver)
		walkTrunk(n.Increment, currentFunc, loopDepth+1, false, infoMap, resolver)
		walkTrunk(n.Body, currentFunc, loopDepth+1, false, infoMap, resolver)
	case *ForRangeStatement:
		walkTrunk(n.RangeValue, currentFunc, loopDepth, false, infoMap, resolver)
		walkTrunk(n.Body, currentFunc, loopDepth+1, false, infoMap, resolver)
	case *CallExpression:
		target := resolver(n.Function)
		if target != nil {
			if info, ok := infoMap[target]; ok {
				if loopDepth > 0 {
					info.loopCall = true
				}
				info.callsites = append(info.callsites, currentFunc)
			}
		}
		// Walk the function expression in case it's a dynamic call returning a function, etc.
		// We pass inCallFunc = true to signal that this expression is the direct target of a call.
		walkTrunk(n.Function, currentFunc, loopDepth, true, infoMap, resolver)
		for _, arg := range n.Arguments {
			walkTrunk(arg, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *AssignStatement:
		for _, v := range n.Values {
			walkTrunk(v, currentFunc, loopDepth, false, infoMap, resolver)
		}
		for _, name := range n.Names {
			walkTrunk(name, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *OpAssignStatement:
		walkTrunk(n.Name, currentFunc, loopDepth, false, infoMap, resolver)
		walkTrunk(n.Value, currentFunc, loopDepth, false, infoMap, resolver)
	case *VarStatement:
		walkTrunk(n.Value, currentFunc, loopDepth, false, infoMap, resolver)
	case *ReturnStatement:
		for _, v := range n.ReturnValues {
			walkTrunk(v, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *ExpressionStatement:
		walkTrunk(n.Expression, currentFunc, loopDepth, false, infoMap, resolver)
	case *IncDecStatement:
		walkTrunk(n.Name, currentFunc, loopDepth, false, infoMap, resolver)
	case *PrefixExpression:
		walkTrunk(n.Right, currentFunc, loopDepth, false, infoMap, resolver)
	case *InfixExpression:
		walkTrunk(n.Left, currentFunc, loopDepth, false, infoMap, resolver)
		walkTrunk(n.Right, currentFunc, loopDepth, false, infoMap, resolver)
	case *IndexExpression:
		if !inCallFunc {
			// e.g. a dynamic reference disguised as a generic instantiation
			if target := resolver(n); target != nil {
				if info, ok := infoMap[target]; ok {
					info.dynamic = true
				}
			}
		}
		walkTrunk(n.Left, currentFunc, loopDepth, false, infoMap, resolver)
		for _, idx := range n.Indices {
			walkTrunk(idx, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *SelectorExpression:
		if !inCallFunc {
			if target := resolver(n); target != nil {
				if info, ok := infoMap[target]; ok {
					info.dynamic = true
				}
			}
		}
		walkTrunk(n.Left, currentFunc, loopDepth, false, infoMap, resolver)
	case *Identifier:
		if !inCallFunc {
			if target := resolver(n); target != nil {
				if info, ok := infoMap[target]; ok {
					info.dynamic = true
				}
			}
		}
	case *PointerType:
		walkTrunk(n.Elt, currentFunc, loopDepth, false, infoMap, resolver)
	case *ArrayType:
		walkTrunk(n.Length, currentFunc, loopDepth, false, infoMap, resolver)
		walkTrunk(n.Elt, currentFunc, loopDepth, false, infoMap, resolver)
	case *DeferStatement:
		if n.Block != nil {
			walkTrunk(n.Block, currentFunc, loopDepth, false, infoMap, resolver)
		} else {
			walkTrunk(n.Call, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *CompositeLit:
		walkTrunk(n.Type, currentFunc, loopDepth, false, infoMap, resolver)
		for _, elt := range n.Elements {
			walkTrunk(elt, currentFunc, loopDepth, false, infoMap, resolver)
		}
	case *KeyValueExpr:
		walkTrunk(n.Key, currentFunc, loopDepth, false, infoMap, resolver)
		walkTrunk(n.Value, currentFunc, loopDepth, false, infoMap, resolver)
		// Add other necessary expression types...
	}
}
