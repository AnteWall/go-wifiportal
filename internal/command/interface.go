package command

import (
	"context"
	"time"
)

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type Runner interface {
	Run(cmd string, args ...string) (Result, error)
	RunWithContext(ctx context.Context, cmd string, args ...string) (Result, error)
	RunWithTimeout(timeout time.Duration, cmd string, args ...string) (Result, error)
}
