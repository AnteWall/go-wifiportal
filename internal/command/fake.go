package command

import (
	"context"
	"time"
)

type FakeRunner struct {
	Scripts map[string]Result
}

func (f *FakeRunner) Run(cmd string, args ...string) (Result, error) {
	key := cmd
	for _, arg := range args {
		key += " " + arg
	}
	if result, ok := f.Scripts[key]; ok {
		return result, nil
	}
	return Result{}, nil
}

func (f *FakeRunner) RunWithContext(ctx context.Context, cmd string, args ...string) (Result, error) {
	return f.Run(cmd, args...)
}

func (f *FakeRunner) RunWithTimeout(timeout time.Duration, cmd string, args ...string) (Result, error) {
	return f.Run(cmd, args...)
}

func (f *FakeRunner) AddScript(cmd string, args []string, result Result) {
	key := cmd
	for _, arg := range args {
		key += " " + arg
	}
	f.Scripts[key] = result
}

func NewFakeRunner() *FakeRunner {
	return &FakeRunner{
		Scripts: make(map[string]Result),
	}
}
