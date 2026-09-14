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
					out[prevIdx] = "\ttfr d,x\t; peephole: pshs d + puls x"
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
									out[prevIdx] = fmt.Sprintf("\tleas %d,s", sum)
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
				if strings.HasPrefix(codePart, "ldd ") && strings.HasPrefix(prevCode, "stx ") {
					if codePart[4:] == prevCode[4:] {
						out = append(out, "\ttfr x,d\t; peephole: stx+ldd -> tfr x,d")
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
									out[prev2Idx] = fmt.Sprintf("\t%s %s\t; peephole: inverted branch over jump", invOp, target2)
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
