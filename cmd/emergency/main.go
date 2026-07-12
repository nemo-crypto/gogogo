package main

import (
	"flag"
	"fmt"
	"log"

	"gogogo/internal/runtime"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	action := flag.String("action", "status", "status, halt, resume")
	path := flag.String("path", ".runtime/halt", "halt file path")
	flag.Parse()

	guard := runtime.NewGuard(*path)
	switch *action {
	case "halt":
		if err := guard.Halt(); err != nil {
			return err
		}
	case "resume":
		if err := guard.Resume(); err != nil {
			return err
		}
	case "status":
	default:
		return fmt.Errorf("unsupported action %q", *action)
	}
	fmt.Printf("emergency_halted=%t path=%s\n", guard.Halted(), guard.HaltFile)
	return nil
}
