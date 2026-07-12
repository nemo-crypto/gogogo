package runtime

import (
	"errors"
	"os"
	"path/filepath"
)

type Guard struct {
	HaltFile string
}

func NewGuard(path string) Guard {
	if path == "" {
		path = ".runtime/halt"
	}
	return Guard{HaltFile: path}
}

func (g Guard) Halted() bool {
	_, err := os.Stat(g.HaltFile)
	return err == nil
}

func (g Guard) Halt() error {
	if err := os.MkdirAll(filepath.Dir(g.HaltFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(g.HaltFile, []byte("halted\n"), 0o644)
}

func (g Guard) Resume() error {
	if err := os.Remove(g.HaltFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
