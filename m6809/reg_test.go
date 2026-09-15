package m6809

import (
	"testing"
)

func TestRegMaskOverlapsAndContains(t *testing.T) {
	// D overlaps with A and B
	if !RegD.Overlaps(RegA) {
		t.Errorf("RegD should overlap with RegA")
	}
	if !RegD.Overlaps(RegB) {
		t.Errorf("RegD should overlap with RegB")
	}
	if RegD.Overlaps(RegX) {
		t.Errorf("RegD should not overlap with RegX")
	}

	// D contains both A and B
	if !RegD.Contains(RegA) {
		t.Errorf("RegD should contain RegA")
	}
	if !RegD.Contains(RegB) {
		t.Errorf("RegD should contain RegB")
	}
	if RegA.Contains(RegD) {
		t.Errorf("RegA should not contain RegD")
	}

	// Disjoint index registers
	if RegX.Overlaps(RegY) {
		t.Errorf("RegX should not overlap with RegY")
	}
	if RegX.Overlaps(RegU) {
		t.Errorf("RegX should not overlap with RegU")
	}
}

func TestAllocatableRegistersVariants(t *testing.T) {
	// Default variant: all registers (A, B, X, Y, U) allocatable
	def := AllocatableRegisters(false, false)
	if !def.Contains(RegA) || !def.Contains(RegB) || !def.Contains(RegX) || !def.Contains(RegY) || !def.Contains(RegU) {
		t.Errorf("Default variant should have A, B, X, Y, U allocatable, got %s", def.String())
	}

	// globalsAtY: Y must NOT be allocatable
	gay := AllocatableRegisters(true, false)
	if gay.Overlaps(RegY) {
		t.Errorf("globalsAtY should exclude RegY")
	}
	if !gay.Contains(RegX) || !gay.Contains(RegU) {
		t.Errorf("globalsAtY should still allow X and U")
	}

	// framePointer: U must NOT be allocatable
	fp := AllocatableRegisters(false, true)
	if fp.Overlaps(RegU) {
		t.Errorf("framePointer should exclude RegU")
	}
	if !fp.Contains(RegX) || !fp.Contains(RegY) {
		t.Errorf("framePointer should still allow X and Y")
	}

	// Both globalsAtY and framePointer: only A, B, X available
	both := AllocatableRegisters(true, true)
	if both.Overlaps(RegY) || both.Overlaps(RegU) {
		t.Errorf("both flags should exclude both RegY and RegU")
	}
	if !both.Contains(RegX) || !both.Contains(RegA) || !both.Contains(RegB) {
		t.Errorf("both flags should still allow X, A, B")
	}
}

func TestAssemblyNameAndSize(t *testing.T) {
	if AssemblyName(RegA) != "a" || AssemblyName(RegB) != "b" || AssemblyName(RegD) != "d" {
		t.Errorf("Incorrect accumulator names")
	}
	if AssemblyName(RegX) != "x" || AssemblyName(RegY) != "y" || AssemblyName(RegU) != "u" {
		t.Errorf("Incorrect index register names")
	}

	if RegisterSize(RegA) != 1 || RegisterSize(RegB) != 1 {
		t.Errorf("A and B must be 1 byte")
	}
	if RegisterSize(RegD) != 2 || RegisterSize(RegX) != 2 || RegisterSize(RegY) != 2 || RegisterSize(RegU) != 2 {
		t.Errorf("D, X, Y, U must be 2 bytes")
	}
}
