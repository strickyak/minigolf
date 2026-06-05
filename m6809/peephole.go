package m6809

import (
	"flag"
	"strings"
)

var DisableTrivialMath = flag.Bool("disable_trivial_math", false, "disable trivial math and offset elimination peephole optimizations")

func peepholeOptimize(asm string) string {
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
				if codePart == "addd #0" || codePart == "subd #0" || codePart == "leax 0,x" || codePart == "leau 0,u" {
					changed = true
					continue
				}
			}

			var prevCode string
			prevIdx := len(out) - 1
			for prevIdx >= 0 {
				pt := strings.TrimSpace(out[prevIdx])
				if idx := strings.Index(pt, ";"); idx != -1 {
					pt = strings.TrimSpace(pt[:idx])
				}
				if pt != "" {
					prevCode = pt
					break
				}
				prevIdx--
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

				// Redundant Load/Store
				if strings.HasPrefix(codePart, "ldd ") && strings.HasPrefix(prevCode, "std ") {
					if codePart[4:] == prevCode[4:] {
						changed = true
						continue // redundant load
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
			if idx := strings.Index(trimmed, ".L_"); idx != -1 {
				target := trimmed[idx:]
				if spaceIdx := strings.IndexAny(target, " \t,"); spaceIdx != -1 {
					target = target[:spaceIdx]
				}
				usedLabels[target] = true
			}
		}

		var finalOut []string
		for _, line := range out {
			trimmed := strings.TrimSpace(line)
			if idx := strings.Index(trimmed, ";"); idx != -1 {
				trimmed = strings.TrimSpace(trimmed[:idx])
			}

			if strings.HasSuffix(trimmed, ":") && strings.HasPrefix(trimmed, ".L_") {
				label := trimmed[:len(trimmed)-1]
				if !usedLabels[label] {
					changed = true
					continue // Skip unused label
				}
			}

			finalOut = append(finalOut, line)
		}

		asm = strings.Join(finalOut, "\n")
		if !changed {
			return asm
		}
	}
}
