package runtime

import (
	"path/filepath"
	"testing"
)

func TestGuardHaltAndResume(t *testing.T) {
	t.Parallel()

	guard := NewGuard(filepath.Join(t.TempDir(), "halt"))
	if guard.Halted() {
		t.Fatal("halted = true, want false")
	}
	if err := guard.Halt(); err != nil {
		t.Fatalf("halt: %v", err)
	}
	if !guard.Halted() {
		t.Fatal("halted = false, want true")
	}
	if err := guard.Resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if guard.Halted() {
		t.Fatal("halted = true after resume, want false")
	}
}
