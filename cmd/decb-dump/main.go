package main

import (
	"flag"
	"fmt"
	"os"
)

var hexFlag = flag.Bool("x", false, "Print hexdump of section payloads")

func sectionTypeName(t byte, addr uint16) string {
	switch t {
	case 0x00:
		return "DATA"
	case 0xFD:
		c1 := byte(addr >> 8)
		c2 := byte(addr & 0xFF)
		if c1 >= 0x20 && c1 <= 0x7E && c2 >= 0x20 && c2 <= 0x7E {
			return fmt.Sprintf("MAGIC (%q)", string([]byte{c1, c2}))
		}
		return "MAGIC"
	case 0xFE:
		return "HIGH_ADDR"
	case 0xFF:
		return "END/EXEC"
	default:
		return "UNKNOWN"
	}
}

func dumpPayload(payload []byte, startAddr uint32, high16 uint16) {
	for offset := 0; offset < len(payload); offset += 16 {
		rowAddr := startAddr + uint32(offset)
		end := offset + 16
		if end > len(payload) {
			end = len(payload)
		}
		row := payload[offset:end]

		if high16 != 0 {
			fmt.Printf("  %08X: ", rowAddr)
		} else {
			fmt.Printf("  %04X: ", rowAddr&0xFFFF)
		}

		for i := 0; i < 16; i++ {
			if i == 8 {
				fmt.Print(" ")
			}
			if i < len(row) {
				fmt.Printf("%02X ", row[i])
			} else {
				fmt.Print("   ")
			}
		}

		fmt.Print(" |")
		for i := 0; i < len(row); i++ {
			b := row[i]
			if b >= 0x20 && b <= 0x7E {
				fmt.Printf("%c", b)
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println("|")
	}
}

func dumpDECBFile(filename string, showHex bool) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		fmt.Printf("[ERROR: empty input file]\n")
		return fmt.Errorf("%s: empty input file", filename)
	}

	pos := 0
	secIdx := 0
	var curHigh16 uint16 = 0

	for pos < len(data) {
		if pos+5 > len(data) {
			rem := data[pos:]
			fmt.Printf("Section %d [offset 0x%04X]: [ERROR: truncated header (%d trailing bytes: % X)]\n",
				secIdx, pos, len(rem), rem)
			return fmt.Errorf("%s: truncated header at offset 0x%04X", filename, pos)
		}

		hdrOffset := pos
		secType := data[pos]
		length := (uint16(data[pos+1]) << 8) | uint16(data[pos+2])
		addr := (uint16(data[pos+3]) << 8) | uint16(data[pos+4])
		pos += 5

		typeName := sectionTypeName(secType, addr)
		fullAddr := (uint32(curHigh16) << 16) | uint32(addr)

		if secType == 0xFE {
			// Section 0xFE sets the high 16 bits of address for following sections.
			fmt.Printf("Section %d [offset 0x%04X] hdr=[%02X %02X %02X %02X %02X]: type=0x%02X (%s) len=0x%04X (%d) addr=0x%04X (high16=0x%04X)\n",
				secIdx, hdrOffset,
				secType, data[hdrOffset+1], data[hdrOffset+2], data[hdrOffset+3], data[hdrOffset+4],
				secType, typeName, length, length, addr, addr)
		} else if curHigh16 != 0 {
			if length > 0 {
				limit := fullAddr + uint32(length)
				fmt.Printf("Section %d [offset 0x%04X] hdr=[%02X %02X %02X %02X %02X]: type=0x%02X (%s) len=0x%04X (%d) addr=0x%08X limit=0x%08X\n",
					secIdx, hdrOffset,
					secType, data[hdrOffset+1], data[hdrOffset+2], data[hdrOffset+3], data[hdrOffset+4],
					secType, typeName, length, length, fullAddr, limit)
			} else {
				fmt.Printf("Section %d [offset 0x%04X] hdr=[%02X %02X %02X %02X %02X]: type=0x%02X (%s) len=0x%04X (%d) addr=0x%08X\n",
					secIdx, hdrOffset,
					secType, data[hdrOffset+1], data[hdrOffset+2], data[hdrOffset+3], data[hdrOffset+4],
					secType, typeName, length, length, fullAddr)
			}
		} else {
			if length > 0 {
				limit := fullAddr + uint32(length)
				fmt.Printf("Section %d [offset 0x%04X] hdr=[%02X %02X %02X %02X %02X]: type=0x%02X (%s) len=0x%04X (%d) addr=0x%04X limit=0x%04X\n",
					secIdx, hdrOffset,
					secType, data[hdrOffset+1], data[hdrOffset+2], data[hdrOffset+3], data[hdrOffset+4],
					secType, typeName, length, length, addr, limit)
			} else {
				fmt.Printf("Section %d [offset 0x%04X] hdr=[%02X %02X %02X %02X %02X]: type=0x%02X (%s) len=0x%04X (%d) addr=0x%04X\n",
					secIdx, hdrOffset,
					secType, data[hdrOffset+1], data[hdrOffset+2], data[hdrOffset+3], data[hdrOffset+4],
					secType, typeName, length, length, addr)
			}
		}

		if length > 0 {
			end := pos + int(length)
			truncated := false
			if end > len(data) {
				end = len(data)
				truncated = true
			}
			payload := data[pos:end]
			if showHex {
				dumpPayload(payload, fullAddr, curHigh16)
			}
			if truncated {
				fmt.Printf("  [ERROR: payload truncated at EOF: got %d of %d bytes]\n", len(payload), length)
				return fmt.Errorf("%s: payload truncated at EOF (got %d of %d bytes)", filename, len(payload), length)
			}
			pos = end
		}

		if secType == 0xFE {
			curHigh16 = addr
		}

		secIdx++
	}

	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: decb-dump [-x] <file.decb> [file2.decb ...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	for i, arg := range flag.Args() {
		if flag.NArg() > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("=== %s ===\n", arg)
		}
		if err := dumpDECBFile(arg, *hexFlag); err != nil {
			fmt.Fprintf(os.Stderr, "decb-dump: %v\n", err)
			os.Exit(1)
		}
	}
}
