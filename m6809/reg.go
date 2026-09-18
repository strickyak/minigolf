package m6809

import (
	"fmt"
	"strings"
)

// RegMask represents physical registers and combinations as a bitmask.
// This directly models register aliasing (e.g., RegD = RegA | RegB).
type RegMask uint16

const (
	RegNone RegMask = 0
	RegA    RegMask = 1 << 0      // High byte accumulator (8-bit)
	RegB    RegMask = 1 << 1      // Low byte accumulator (8-bit)
	RegD    RegMask = RegA | RegB // Composite 16-bit accumulator (A:B)
	RegX    RegMask = 1 << 2      // Index register X (16-bit)
	RegY    RegMask = 1 << 3      // Index register Y (16-bit)
	RegU    RegMask = 1 << 4      // User stack pointer / Index register U (16-bit)
	RegS    RegMask = 1 << 5      // System stack pointer (16-bit, reserved)
	RegCC   RegMask = 1 << 6      // Condition Code register (8-bit)
)

const (
	// Register Classes
	ClassAcc8   RegMask = RegA | RegB
	ClassAcc16  RegMask = RegD
	ClassIndex  RegMask = RegX | RegY | RegU
	ClassAll16  RegMask = RegD | RegX | RegY | RegU
	ClassAllReg RegMask = RegA | RegB | RegX | RegY | RegU
)

// Overlaps returns true if any physical register resource is shared.
func (r RegMask) Overlaps(other RegMask) bool {
	return (r & other) != 0
}

// Contains returns true if r contains all the resources of other.
func (r RegMask) Contains(other RegMask) bool {
	return (r & other) == other
}

// String returns a human-readable representation of the register(s).
func (r RegMask) String() string {
	if r == RegNone {
		return "none"
	}
	if r == RegD {
		return "d"
	}
	var parts []string
	if r&RegA != 0 && r&RegD != RegD {
		parts = append(parts, "a")
	}
	if r&RegB != 0 && r&RegD != RegD {
		parts = append(parts, "b")
	}
	if r&RegD == RegD {
		parts = append(parts, "d")
	}
	if r&RegX != 0 {
		parts = append(parts, "x")
	}
	if r&RegY != 0 {
		parts = append(parts, "y")
	}
	if r&RegU != 0 {
		parts = append(parts, "u")
	}
	if r&RegS != 0 {
		parts = append(parts, "s")
	}
	if r&RegCC != 0 {
		parts = append(parts, "cc")
	}
	return strings.Join(parts, "|")
}

// AllocatableRegisters returns the bitmask of registers available for
// allocation given the active compilation variant.
func AllocatableRegisters(globalsAtY, framePointer bool) RegMask {
	mask := ClassAllReg
	if globalsAtY {
		// Y is reserved as base pointer for global data
		mask &^= RegY
	}
	if framePointer {
		// U is reserved as hardware frame pointer
		mask &^= RegU
	}
	return mask
}

// AllocatableIndexRegisters returns the index registers available for
// pointer math and indexing under the current compilation variant.
func AllocatableIndexRegisters(globalsAtY, framePointer bool) RegMask {
	mask := ClassIndex
	if globalsAtY {
		mask &^= RegY
	}
	if framePointer {
		mask &^= RegU
	}
	return mask
}

// RegisterSize returns the size in bytes of the physical register.
func RegisterSize(reg RegMask) int {
	switch reg {
	case RegA, RegB:
		return 1
	case RegD, RegX, RegY, RegU, RegS:
		return 2
	default:
		if reg&RegD == RegD {
			return 2
		}
		if reg&(RegA|RegB) != 0 {
			return 1
		}
		return 2
	}
}

// AssemblyName returns the M6809 assembly register name (lowercase).
func AssemblyName(reg RegMask) string {
	switch reg {
	case RegA:
		return "a"
	case RegB:
		return "b"
	case RegD:
		return "d"
	case RegX:
		return "x"
	case RegY:
		return "y"
	case RegU:
		return "u"
	case RegS:
		return "s"
	case RegCC:
		return "cc"
	default:
		panic(fmt.Sprintf("AssemblyName: invalid single register mask 0x%x", uint16(reg)))
	}
}
