package cdp1802

import (
	"strings"
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestBackendInstantiation(t *testing.T) {
	b := New()
	if b == nil {
		t.Fatal("New() returned nil")
	}

	prog := &ir.Program{
		Functions: []*ir.Function{},
		Globals:   []*ir.Global{},
	}

	code := b.Generate(prog)
	if !strings.Contains(code, "cstart:") {
		t.Errorf("Generated assembly missing cstart: header\n%s", code)
	}
	if !strings.Contains(code, "_CALL:") {
		t.Errorf("Generated assembly missing _CALL engine\n%s", code)
	}
}
