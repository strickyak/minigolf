package m6809

import (
	"flag"
	"strings"
)

var DisableTrivialMath = flag.Bool("disable_trivial_math", false, "disable trivial math and offset elimination peephole optimizations")

func peepholeOptimize(asm string) string {
	lines := strings.Split(asm, "\n")
	var out []string

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
				continue
			}
			if codePart == "puls x" && prevCode == "pshs d" {
				out[prevIdx] = "\ttfr d,x\t; peephole: pshs d + puls x"
				continue
			}

			// Redundant TFR
			if codePart == "tfr x,d" && prevCode == "tfr d,x" {
				continue
			}
			if codePart == "tfr y,d" && prevCode == "tfr d,y" {
				continue
			}
			if codePart == "tfr u,d" && prevCode == "tfr d,u" {
				continue
			}

			// Redundant Load/Store
			if strings.HasPrefix(codePart, "ldd ") && strings.HasPrefix(prevCode, "std ") {
				if codePart[4:] == prevCode[4:] {
					continue // redundant load
				}
			}
			if strings.HasPrefix(codePart, "ldx ") && strings.HasPrefix(prevCode, "stx ") {
				if codePart[4:] == prevCode[4:] {
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
					}
				}
			}
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}
