package command

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

type execRunner struct{}

func (e *execRunner) Run(cmd string, args ...string) (Result, error) {
	command := exec.Command(cmd, args...)
	var out, errBuffer bytes.Buffer
	command.Stdout, command.Stderr = &out, &errBuffer
	runErr := command.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(runErr, &ee) && ee.ProcessState != nil {
		code = ee.ProcessState.ExitCode()
	}
	return Result{Stdout: out.Bytes(), Stderr: errBuffer.Bytes(), ExitCode: code}, runErr
}

func (e *execRunner) RunWithContext(ctx context.Context, cmd string, args ...string) (Result, error) {
	command := exec.CommandContext(ctx, cmd, args...)
	var out, errBuffer bytes.Buffer
	command.Stdout, command.Stderr = &out, &errBuffer
	runErr := command.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(runErr, &ee) && ee.ProcessState != nil {
		code = ee.ProcessState.ExitCode()
	}
	return Result{Stdout: out.Bytes(), Stderr: errBuffer.Bytes(), ExitCode: code}, runErr
}

func (e *execRunner) RunWithTimeout(timeout time.Duration, cmd string, args ...string) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return e.RunWithContext(ctx, cmd, args...)
}

func NewExecRunner() Runner {
	return &execRunner{}
}
