package m6809

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

var DisableTrivialMath = flag.Bool("disable_trivial_math", false, "disable trivial math and offset elimination peephole optimizations")
var noPeephole6809 = flag.Bool("no-peephole6809", false, "disable peephole optimizations on M6809")

var condInverses = map[string]string{
	"beq":  "lbne",
	"bne":  "lbeq",
	"blt":  "lbge",
	"ble":  "lbgt",
	"bgt":  "lble",
	"bge":  "lblt",
	"blo":  "lbhs",
	"bls":  "lbhi",
	"bhi":  "lbls",
	"bhs":  "lblo",
	"lbeq": "lbne",
	"lbne": "lbeq",
	"lblt": "lbge",
	"lble": "lbgt",
	"lbgt": "lble",
	"lbge": "lblt",
	"lblo": "lbhs",
	"lbls": "lbhi",
	"lbhi": "lbls",
	"lbhs": "lblo",
}

func getComment(line string) string {
	if idx := strings.Index(line, ";"); idx != -1 {
		return strings.TrimSpace(line[idx+1:])
	}
	return ""
}

func combineComments(c1, c2 string) string {
	c1 = strings.TrimSpace(c1)
	c2 = strings.TrimSpace(c2)
	if c1 == "" {
		return c2
	}
	if c2 == "" || c1 == c2 {
		return c1
	}
	return c1 + " | " + c2
}

func withComment(code, comment string) string {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return code
	}
	return code + "\t; " + comment
}

func peepholeOptimize(asm string) string {
	if os.Getenv("NO_PEEPHOLE6809") != "" {
		*noPeephole6809 = true
	}
	if *noPeephole6809 {
		return asm
	}

	for {
		lines := strings.Split(asm, "\n")
		var out []string
		changed := false

		for i := 0; i < len(lines); i++ {
			line := lines[i]

			trimmed := strings.TrimSpace(line)

			codePart := trimmed
			if idx := strings.Index(codePart, ";"); idx != -1 {
				codePart = strings.TrimSpace(codePart[:idx])
			}

			if codePart == "" {
				out = append(out, line)
				continue
			}

			if !*DisableTrivialMath {
				if codePart == "addd #0" || codePart == "subd #0" || codePart == "leax 0,x" || codePart == "leau 0,u" || codePart == "leas 0,s" {
					changed = true
					continue
				}
			}

			var prevCode, prev2Code string
			prevIdx := -1
			prev2Idx := -1
			p := len(out) - 1
			for p >= 0 {
				pt := strings.TrimSpace(out[p])
				if idx := strings.Index(pt, ";"); idx != -1 {
					pt = strings.TrimSpace(pt[:idx])
				}
				if pt != "" {
					if prevIdx == -1 {
						prevIdx = p
						prevCode = pt
					} else if prev2Idx == -1 {
						prev2Idx = p
						prev2Code = pt
						break
					}
				}
				p--
			}

			if prevIdx >= 0 {
				// Push/Pull cancellation
				if codePart == "puls d" && prevCode == "pshs d" {
					out = append(out[:prevIdx], out[prevIdx+1:]...) // remove pshs but keep any comments in between
					changed = true
					continue
				}
				if codePart == "puls x" && prevCode == "pshs d" {
					c := combineComments(getComment(out[prevIdx]), getComment(line))
					out[prevIdx] = withComment("\ttfr d,x", combineComments(c, "peephole: pshs d + puls x"))
					changed = true
					continue
				}

				// Stack adjustment combination
				if strings.HasPrefix(codePart, "leas ") && strings.HasPrefix(prevCode, "leas ") {
					if strings.HasSuffix(codePart, ",s") && strings.HasSuffix(prevCode, ",s") {
						var n, m int
						if _, err1 := fmt.Sscanf(codePart, "leas %d,s", &n); err1 == nil {
							if _, err2 := fmt.Sscanf(prevCode, "leas %d,s", &m); err2 == nil {
								sum := n + m
								if sum == 0 {
									out = append(out[:prevIdx], out[prevIdx+1:]...)
									changed = true
									continue
								} else {
									c := combineComments(getComment(out[prevIdx]), getComment(line))
									out[prevIdx] = withComment(fmt.Sprintf("\tleas %d,s", sum), c)
									changed = true
									continue
								}
							}
						}
					}
				}

				// Redundant TFR
				if codePart == "tfr x,d" && prevCode == "tfr d,x" {
					changed = true
					continue
				}
				if codePart == "tfr y,d" && prevCode == "tfr d,y" {
					changed = true
					continue
				}
				if codePart == "tfr u,d" && prevCode == "tfr d,u" {
					changed = true
					continue
				}
				if codePart == "tfr d,d" || codePart == "tfr x,x" || codePart == "tfr y,y" || codePart == "tfr u,u" || codePart == "tfr s,s" {
					changed = true
					continue
				}


				// Redundant TST
				if codePart == "tstb" {
					if strings.HasPrefix(prevCode, "ldb ") || strings.HasPrefix(prevCode, "stb ") ||
						strings.HasPrefix(prevCode, "addb ") || strings.HasPrefix(prevCode, "subb ") ||
						strings.HasPrefix(prevCode, "andb ") || strings.HasPrefix(prevCode, "orb ") ||
						strings.HasPrefix(prevCode, "eorb ") || strings.HasPrefix(prevCode, "negb") ||
						prevCode == "clrb" {
						changed = true
						continue
					}
				}
				if codePart == "tsta" {
					if strings.HasPrefix(prevCode, "lda ") || strings.HasPrefix(prevCode, "sta ") ||
						strings.HasPrefix(prevCode, "adda ") || strings.HasPrefix(prevCode, "suba ") ||
						strings.HasPrefix(prevCode, "anda ") || strings.HasPrefix(prevCode, "ora ") ||
						strings.HasPrefix(prevCode, "eora ") || strings.HasPrefix(prevCode, "nega") ||
						prevCode == "clra" {
						changed = true
						continue
					}
				}

				// Redundant Store
				if strings.HasPrefix(codePart, "std ") || strings.HasPrefix(codePart, "stb ") || strings.HasPrefix(codePart, "sta ") || strings.HasPrefix(codePart, "stx ") || strings.HasPrefix(codePart, "sty ") || strings.HasPrefix(codePart, "stu ") {
					if codePart == prevCode {
						changed = true
						continue // redundant store to same address
					}
				}

				// Redundant Load/Store
				if strings.HasPrefix(codePart, "ldd ") && strings.HasPrefix(prevCode, "std ") {
					if codePart[4:] == prevCode[4:] {
						changed = true
						continue // redundant load
					}
				}
				if strings.HasPrefix(codePart, "ldx ") && strings.HasPrefix(prevCode, "std ") {
					if codePart[4:] == prevCode[4:] {
						out = append(out, "\ttfr d,x\t; peephole: std+ldx -> tfr d,x")
						changed = true
						continue
					}
				}
				if strings.HasPrefix(codePart, "ldy ") && strings.HasPrefix(prevCode, "std ") {
					if codePart[4:] == prevCode[4:] {
						out = append(out, "\ttfr d,y\t; peephole: std+ldy -> tfr d,y")
						changed = true
						continue
					}
				}
				if strings.HasPrefix(codePart, "ldu ") && strings.HasPrefix(prevCode, "std ") {
					if codePart[4:] == prevCode[4:] {
						out = append(out, "\ttfr d,u\t; peephole: std+ldu -> tfr d,u")
						changed = true
						continue
					}
				}
				if strings.HasPrefix(codePart, "ldd ") && strings.HasPrefix(prevCode, "stx ") {
					if codePart[4:] == prevCode[4:] {
						out = append(out, "\ttfr x,d\t; peephole: stx+ldd -> tfr x,d")
						changed = true
						continue
					}
				}
				if strings.HasPrefix(codePart, "ldd ") && strings.HasPrefix(prevCode, "sty ") {
					if codePart[4:] == prevCode[4:] {
						out = append(out, "\ttfr y,d\t; peephole: sty+ldd -> tfr y,d")
						changed = true
						continue
					}
				}
				if strings.HasPrefix(codePart, "ldd ") && strings.HasPrefix(prevCode, "stu ") {
					if codePart[4:] == prevCode[4:] {
						out = append(out, "\ttfr u,d\t; peephole: stu+ldd -> tfr u,d")
						changed = true
						continue
					}
				}
				if strings.HasPrefix(codePart, "ldx ") && strings.HasPrefix(prevCode, "stx ") {
					if codePart[4:] == prevCode[4:] {
						changed = true
						continue // redundant load
					}
				}
				if strings.HasPrefix(codePart, "ldy ") && strings.HasPrefix(prevCode, "sty ") {
					if codePart[4:] == prevCode[4:] {
						changed = true
						continue // redundant load
					}
				}
				if strings.HasPrefix(codePart, "ldu ") && strings.HasPrefix(prevCode, "stu ") {
					if codePart[4:] == prevCode[4:] {
						changed = true
						continue // redundant load
					}
				}
				if strings.HasPrefix(codePart, "ldb ") && strings.HasPrefix(prevCode, "stb ") {
					if codePart[4:] == prevCode[4:] {
						changed = true
						continue // redundant load
					}
				}
				if strings.HasPrefix(codePart, "lda ") && strings.HasPrefix(prevCode, "sta ") {
					if codePart[4:] == prevCode[4:] {
						changed = true
						continue // redundant load
					}
				}

				// Redundant load after store across non-modifying conditional branch
				if prev2Idx >= 0 {
					pCode := prevCode
					if idx := strings.Index(pCode, " "); idx != -1 {
						pOp := pCode[:idx]
						switch pOp {
						case "beq", "lbeq", "bne", "lbne", "bgt", "lbgt", "blt", "lblt",
							"bge", "lbge", "ble", "lble", "blo", "lblo", "bhi", "lbhi",
							"bls", "lbls", "bhs", "lbhs":
							if strings.HasPrefix(codePart, "ldb ") && strings.HasPrefix(prev2Code, "stb ") && codePart[4:] == prev2Code[4:] {
								changed = true
								continue
							}
							if strings.HasPrefix(codePart, "lda ") && strings.HasPrefix(prev2Code, "sta ") && codePart[4:] == prev2Code[4:] {
								changed = true
								continue
							}
							if strings.HasPrefix(codePart, "ldd ") && strings.HasPrefix(prev2Code, "std ") && codePart[4:] == prev2Code[4:] {
								changed = true
								continue
							}
						}
					}
				}

				// Autoincrement / Autodecrement Addressing
				if codePart == "leax 1,x" {
					if prevCode == "ldb ,x" || prevCode == "stb ,x" || prevCode == "lda ,x" || prevCode == "sta ,x" {
						c := combineComments(getComment(out[prevIdx]), "peephole: auto-increment")
						out[prevIdx] = withComment("\t"+prevCode[:3]+" ,x+", c)
						changed = true
						continue
					}
				}
				if codePart == "leax 2,x" {
					if prevCode == "ldd ,x" || prevCode == "std ,x" {
						c := combineComments(getComment(out[prevIdx]), "peephole: auto-increment")
						out[prevIdx] = withComment("\t"+prevCode[:3]+" ,x++", c)
						changed = true
						continue
					}
				}
				if codePart == "leay 1,y" {
					if prevCode == "ldb ,y" || prevCode == "stb ,y" || prevCode == "lda ,y" || prevCode == "sta ,y" {
						c := combineComments(getComment(out[prevIdx]), "peephole: auto-increment")
						out[prevIdx] = withComment("\t"+prevCode[:3]+" ,y+", c)
						changed = true
						continue
					}
				}
				if codePart == "leay 2,y" {
					if prevCode == "ldd ,y" || prevCode == "std ,y" {
						c := combineComments(getComment(out[prevIdx]), "peephole: auto-increment")
						out[prevIdx] = withComment("\t"+prevCode[:3]+" ,y++", c)
						changed = true
						continue
					}
				}
				if codePart == "leau 1,u" {
					if prevCode == "ldb ,u" || prevCode == "stb ,u" || prevCode == "lda ,u" || prevCode == "sta ,u" {
						c := combineComments(getComment(out[prevIdx]), "peephole: auto-increment")
						out[prevIdx] = withComment("\t"+prevCode[:3]+" ,u+", c)
						changed = true
						continue
					}
				}
				if codePart == "leau 2,u" {
					if prevCode == "ldd ,u" || prevCode == "std ,u" {
						c := combineComments(getComment(out[prevIdx]), "peephole: auto-increment")
						out[prevIdx] = withComment("\t"+prevCode[:3]+" ,u++", c)
						changed = true
						continue
					}
				}
				if prevCode == "leax -1,x" {
					if codePart == "ldb ,x" || codePart == "stb ,x" || codePart == "lda ,x" || codePart == "sta ,x" {
						c := combineComments(getComment(line), "peephole: auto-decrement")
						out = append(out[:prevIdx], out[prevIdx+1:]...) // remove leax
						out = append(out, withComment("\t"+codePart[:3]+" ,-x", c))
						changed = true
						continue
					}
				}
				if prevCode == "leax -2,x" {
					if codePart == "ldd ,x" || codePart == "std ,x" {
						out = append(out[:prevIdx], out[prevIdx+1:]...) // remove leax
						out = append(out, "\t"+codePart[:3]+" ,--x\t; peephole: auto-decrement")
						changed = true
						continue
					}
				}
				if prevCode == "leay -1,y" {
					if codePart == "ldb ,y" || codePart == "stb ,y" || codePart == "lda ,y" || codePart == "sta ,y" {
						c := combineComments(getComment(line), "peephole: auto-decrement")
						out = append(out[:prevIdx], out[prevIdx+1:]...) // remove leay
						out = append(out, withComment("\t"+codePart[:3]+" ,-y", c))
						changed = true
						continue
					}
				}
				if prevCode == "leay -2,y" {
					if codePart == "ldd ,y" || codePart == "std ,y" {
						out = append(out[:prevIdx], out[prevIdx+1:]...) // remove leay
						out = append(out, "\t"+codePart[:3]+" ,--y\t; peephole: auto-decrement")
						changed = true
						continue
					}
				}
				if prevCode == "leau -1,u" {
					if codePart == "ldb ,u" || codePart == "stb ,u" || codePart == "lda ,u" || codePart == "sta ,u" {
						c := combineComments(getComment(line), "peephole: auto-decrement")
						out = append(out[:prevIdx], out[prevIdx+1:]...) // remove leau
						out = append(out, withComment("\t"+codePart[:3]+" ,-u", c))
						changed = true
						continue
					}
				}
				if prevCode == "leau -2,u" {
					if codePart == "ldd ,u" || codePart == "std ,u" {
						out = append(out[:prevIdx], out[prevIdx+1:]...) // remove leau
						out = append(out, "\t"+codePart[:3]+" ,--u\t; peephole: auto-decrement")
						changed = true
						continue
					}
				}

				// Branch to Next Instruction
				if strings.HasPrefix(prevCode, "bra ") || strings.HasPrefix(prevCode, "lbra ") || strings.HasPrefix(prevCode, "jmp ") {
					fields := strings.Fields(prevCode)
					if len(fields) >= 2 {
						target := fields[1]
						if codePart == target+":" {
							out = append(out[:prevIdx], out[prevIdx+1:]...) // remove branch
							changed = true
						}
					}
				}

				// Conditional Branch Inversion over Jump:
				//   b<cond> L1
				//   (bra|lbra) L2
				// L1:
				//   -> lb<inv_cond> L2
				//      L1:
				if prev2Idx >= 0 && strings.HasSuffix(codePart, ":") {
					label := codePart[:len(codePart)-1]
					if strings.HasPrefix(prevCode, "bra ") || strings.HasPrefix(prevCode, "lbra ") {
						prevFields := strings.Fields(prevCode)
						prev2Fields := strings.Fields(prev2Code)
						if len(prevFields) >= 2 && len(prev2Fields) >= 2 {
							target2 := prevFields[1]
							condOp := prev2Fields[0]
							target1 := prev2Fields[1]
							if target1 == label {
								if invOp, ok := condInverses[condOp]; ok {
									c := combineComments(getComment(out[prev2Idx]), "peephole: inverted branch over jump")
									out[prev2Idx] = withComment(fmt.Sprintf("\t%s %s", invOp, target2), c)
									out = append(out[:prevIdx], out[prevIdx+1:]...) // remove prevCode (lbra/bra)
									changed = true
								}
							}
						}
					}
				}
			}

			out = append(out, line)
		}

		// Pass 2: Unused Label Elimination
		// Re-parse 'out' to find used labels after peepholes have run.
		usedLabels := make(map[string]bool)
		for _, line := range out {
			trimmed := strings.TrimSpace(line)
			if idx := strings.Index(trimmed, ";"); idx != -1 {
				trimmed = strings.TrimSpace(trimmed[:idx])
			}
			if trimmed == "" || strings.HasSuffix(trimmed, ":") {
				continue
			}
			for _, word := range strings.Fields(trimmed) {
				w := strings.TrimLeft(word, "#,[]")
				if strings.HasPrefix(w, ".L") {
					if spaceIdx := strings.IndexAny(w, " \t,"); spaceIdx != -1 {
						w = w[:spaceIdx]
					}
					usedLabels[w] = true
				}
			}
		}

		var finalOut []string
		for _, line := range out {
			trimmed := strings.TrimSpace(line)
			if idx := strings.Index(trimmed, ";"); idx != -1 {
				trimmed = strings.TrimSpace(trimmed[:idx])
			}

			if strings.HasSuffix(trimmed, ":") && (strings.HasPrefix(trimmed, ".L_") || strings.HasPrefix(trimmed, ".LL")) {
				label := trimmed[:len(trimmed)-1]
				if !usedLabels[label] {
					changed = true
					continue // Skip unused label
				}
			}

			finalOut = append(finalOut, line)
		}

		var finalOut2 []string
		// Pass 3: Jump Threading (Jump to Jump optimization)
		labelTargets := make(map[string]string)
		for i := 0; i < len(finalOut); i++ {
			trimmed := strings.TrimSpace(finalOut[i])
			if idx := strings.Index(trimmed, ";"); idx != -1 {
				trimmed = strings.TrimSpace(trimmed[:idx])
			}
			if strings.HasSuffix(trimmed, ":") {
				label := trimmed[:len(trimmed)-1]
				for j := i + 1; j < len(finalOut); j++ {
					nextTrimmed := strings.TrimSpace(finalOut[j])
					if idx := strings.Index(nextTrimmed, ";"); idx != -1 {
						nextTrimmed = strings.TrimSpace(nextTrimmed[:idx])
					}
					if nextTrimmed == "" {
						continue
					}
					if strings.HasPrefix(nextTrimmed, "lbra ") || strings.HasPrefix(nextTrimmed, "bra ") || strings.HasPrefix(nextTrimmed, "jmp ") {
						fields := strings.Fields(nextTrimmed)
						if len(fields) >= 2 {
							target := fields[1]
							if target != label {
								labelTargets[label] = target
							}
						}
					}
					break
				}
			}
		}

		for i, line := range finalOut {
			trimmed := strings.TrimSpace(line)
			if idx := strings.Index(trimmed, ";"); idx != -1 {
				trimmed = strings.TrimSpace(trimmed[:idx])
			}
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				op := fields[0]
				target := fields[1]
				isBranch := false
				switch op {
				case "bra", "lbra", "jmp", "beq", "bne", "blt", "ble", "bgt", "bge", "blo", "bls", "bhi", "bhs", "lbeq", "lbne", "lblt", "lble", "lbgt", "lbge", "lblo", "lbls", "lbhi", "lbhs":
					isBranch = true
				}
				if isBranch {
					if newTarget, ok := labelTargets[target]; ok {
						newOp := op
						if op == "bra" {
							newOp = "lbra"
						} else if op == "jmp" {
							newOp = "jmp"
						} else if !strings.HasPrefix(op, "lb") && op != "lbra" {
							newOp = "l" + op
						}

						newLine := strings.Replace(line, target, newTarget, 1)
						if newOp != op {
							newLine = strings.Replace(newLine, op, newOp, 1)
						}
						finalOut[i] = newLine
						changed = true
					}
				}
			}
		}

		// Pass 4: Unreachable Code Elimination
		unreachable := false
		for _, line := range finalOut {
			trimmed := strings.TrimSpace(line)
			if idx := strings.Index(trimmed, ";"); idx != -1 {
				trimmed = strings.TrimSpace(trimmed[:idx])
			}

			// Labels and data declarations start at column 0 (no indentation)
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				unreachable = false
			}
			if strings.HasSuffix(trimmed, ":") {
				unreachable = false
			}

			if unreachable && trimmed != "" {
				changed = true
				continue // drop unreachable instruction
			}
			if strings.HasPrefix(trimmed, "bra ") || strings.HasPrefix(trimmed, "lbra ") || strings.HasPrefix(trimmed, "jmp ") || trimmed == "rts" || trimmed == "puls u,pc" {
				unreachable = true
			}
			finalOut2 = append(finalOut2, line)
		}

		asm = strings.Join(finalOut2, "\n")
		if !changed {
			return asm
		}
	}
}
