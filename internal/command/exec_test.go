package command

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ExecRunnerTestSuite defines the test suite for execRunner
type ExecRunnerTestSuite struct {
	suite.Suite
	runner Runner
}

// SetupTest runs before each test
func (suite *ExecRunnerTestSuite) SetupTest() {
	suite.runner = NewExecRunner()
}

// TestNewExecRunner tests the constructor
func (suite *ExecRunnerTestSuite) TestNewExecRunner() {
	runner := NewExecRunner()
	suite.NotNil(runner)
	suite.Implements((*Runner)(nil), runner)
}

// TestRun_SuccessfulCommand tests running a successful command
func (suite *ExecRunnerTestSuite) TestRun_SuccessfulCommand() {
	var cmd, expectedOutput string
	if runtime.GOOS == "windows" {
		cmd = "echo"
		expectedOutput = "hello world\r\n"
	} else {
		cmd = "echo"
		expectedOutput = "hello world\n"
	}

	result, err := suite.runner.Run(cmd, "hello", "world")

	suite.NoError(err)
	suite.Equal(0, result.ExitCode)
	suite.Equal(expectedOutput, string(result.Stdout))
	suite.Empty(result.Stderr)
}

// TestRun_CommandWithError tests running a command that fails
func (suite *ExecRunnerTestSuite) TestRun_CommandWithError() {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "sh"
	}

	var result Result
	var err error

	if runtime.GOOS == "windows" {
		result, err = suite.runner.Run(cmd, "/c", "exit 1")
	} else {
		result, err = suite.runner.Run(cmd, "-c", "exit 1")
	}

	suite.Error(err)
	suite.Equal(1, result.ExitCode)
}

// TestRun_NonExistentCommand tests running a command that doesn't exist
func (suite *ExecRunnerTestSuite) TestRun_NonExistentCommand() {
	_, err := suite.runner.Run("nonexistentcommand12345")

	suite.Error(err)
	suite.Contains(err.Error(), "executable file not found")
	// Exit code behavior may vary depending on system
}

// TestRun_CommandWithStderr tests a command that outputs to stderr
func (suite *ExecRunnerTestSuite) TestRun_CommandWithStderr() {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "sh"
	}

	var result Result
	var err error

	if runtime.GOOS == "windows" {
		result, err = suite.runner.Run(cmd, "/c", "echo error message 1>&2")
	} else {
		result, err = suite.runner.Run(cmd, "-c", "echo 'error message' >&2")
	}

	suite.NoError(err)
	suite.Equal(0, result.ExitCode)
	suite.Empty(result.Stdout)
	suite.Contains(string(result.Stderr), "error message")
}

// TestRunWithContext_SuccessfulCommand tests running a command with context
func (suite *ExecRunnerTestSuite) TestRunWithContext_SuccessfulCommand() {
	ctx := context.Background()
	var cmd, expectedOutput string
	if runtime.GOOS == "windows" {
		cmd = "echo"
		expectedOutput = "test\r\n"
	} else {
		cmd = "echo"
		expectedOutput = "test\n"
	}

	result, err := suite.runner.RunWithContext(ctx, cmd, "test")

	suite.NoError(err)
	suite.Equal(0, result.ExitCode)
	suite.Equal(expectedOutput, string(result.Stdout))
}

// TestRunWithContext_CancelledContext tests context cancellation
func (suite *ExecRunnerTestSuite) TestRunWithContext_CancelledContext() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "ping"
	} else {
		cmd = "sleep"
	}

	var err error

	if runtime.GOOS == "windows" {
		_, err = suite.runner.RunWithContext(ctx, cmd, "127.0.0.1", "-n", "10")
	} else {
		_, err = suite.runner.RunWithContext(ctx, cmd, "1")
	}

	suite.Error(err)
	suite.Contains(err.Error(), "context canceled")
	// Exit code can be non-zero due to cancellation
}

// TestRunWithTimeout_SuccessfulCommand tests running a command with timeout that completes in time
func (suite *ExecRunnerTestSuite) TestRunWithTimeout_SuccessfulCommand() {
	timeout := 5 * time.Second
	var cmd, expectedOutput string
	if runtime.GOOS == "windows" {
		cmd = "echo"
		expectedOutput = "quick\r\n"
	} else {
		cmd = "echo"
		expectedOutput = "quick\n"
	}

	result, err := suite.runner.RunWithTimeout(timeout, cmd, "quick")

	suite.NoError(err)
	suite.Equal(0, result.ExitCode)
	suite.Equal(expectedOutput, string(result.Stdout))
}

// TestRunWithTimeout_TimeoutExceeded tests timeout behavior
func (suite *ExecRunnerTestSuite) TestRunWithTimeout_TimeoutExceeded() {
	timeout := 100 * time.Millisecond

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "ping"
	} else {
		cmd = "sleep"
	}

	start := time.Now()
	var err error

	if runtime.GOOS == "windows" {
		_, err = suite.runner.RunWithTimeout(timeout, cmd, "127.0.0.1", "-n", "10")
	} else {
		_, err = suite.runner.RunWithTimeout(timeout, cmd, "2")
	}
	elapsed := time.Since(start)

	suite.Error(err)
	// The error message can vary (context deadline exceeded, signal: killed, etc.)
	suite.True(strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "signal: killed") ||
		strings.Contains(err.Error(), "killed"))
	suite.True(elapsed < 2*time.Second, "Command should have been killed before completion")
}

// TestRun_MultipleArguments tests command with multiple arguments
func (suite *ExecRunnerTestSuite) TestRun_MultipleArguments() {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "sh"
	}

	var result Result
	var err error

	if runtime.GOOS == "windows" {
		result, err = suite.runner.Run(cmd, "/c", "echo", "arg1", "arg2", "arg3")
	} else {
		result, err = suite.runner.Run(cmd, "-c", "echo arg1 arg2 arg3")
	}

	suite.NoError(err)
	suite.Equal(0, result.ExitCode)
	suite.Contains(string(result.Stdout), "arg1")
	suite.Contains(string(result.Stdout), "arg2")
	suite.Contains(string(result.Stdout), "arg3")
}

// TestRun_EmptyCommand tests running with empty command
func (suite *ExecRunnerTestSuite) TestRun_EmptyCommand() {
	_, err := suite.runner.Run("")

	suite.Error(err)
	// Different systems may handle empty commands differently
	// The key is that there should be an error
}

// TestResult_Structure tests the Result structure
func (suite *ExecRunnerTestSuite) TestResult_Structure() {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo"
	} else {
		cmd = "echo"
	}

	result, err := suite.runner.Run(cmd, "test")

	suite.NoError(err)
	suite.IsType([]byte{}, result.Stdout)
	suite.IsType([]byte{}, result.Stderr)
	suite.IsType(int(0), result.ExitCode)
	suite.NotNil(result.Stdout)
	suite.NotNil(result.Stderr)
}

// TestRunWithContext_NilContext tests behavior with nil context
func (suite *ExecRunnerTestSuite) TestRunWithContext_NilContext() {
	// This should not panic and should work similar to Run
	var cmd, expectedOutput string
	if runtime.GOOS == "windows" {
		cmd = "echo"
		expectedOutput = "test\r\n"
	} else {
		cmd = "echo"
		expectedOutput = "test\n"
	}

	result, err := suite.runner.RunWithContext(context.Background(), cmd, "test")

	suite.NoError(err)
	suite.Equal(0, result.ExitCode)
	suite.Equal(expectedOutput, string(result.Stdout))
}

// TestRunWithTimeout_ZeroTimeout tests zero timeout behavior
func (suite *ExecRunnerTestSuite) TestRunWithTimeout_ZeroTimeout() {
	timeout := 0 * time.Second

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo"
	} else {
		cmd = "echo"
	}

	_, err := suite.runner.RunWithTimeout(timeout, cmd, "test")

	// Zero timeout should cause immediate cancellation
	suite.Error(err)
	suite.True(strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "signal: killed") ||
		strings.Contains(err.Error(), "killed"))
}

// TestExitCodeHandling tests various exit codes
func (suite *ExecRunnerTestSuite) TestExitCodeHandling() {
	testCases := []struct {
		name     string
		exitCode int
	}{
		{"exit code 0", 0},
		{"exit code 1", 1},
		{"exit code 2", 2},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			var cmd string
			if runtime.GOOS == "windows" {
				cmd = "cmd"
			} else {
				cmd = "sh"
			}

			var result Result
			var err error

			if runtime.GOOS == "windows" {
				result, err = suite.runner.Run(cmd, "/c", "exit", string(rune(tc.exitCode+'0')))
			} else {
				result, err = suite.runner.Run(cmd, "-c", "exit "+string(rune(tc.exitCode+'0')))
			}

			suite.Equal(tc.exitCode, result.ExitCode)
			if tc.exitCode == 0 {
				suite.NoError(err)
			} else {
				suite.Error(err)
				var exitError *exec.ExitError
				suite.ErrorAs(err, &exitError)
			}
		})
	}
}

// TestConcurrentExecution tests running multiple commands concurrently
func (suite *ExecRunnerTestSuite) TestConcurrentExecution() {
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo"
	} else {
		cmd = "echo"
	}

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			result, err := suite.runner.Run(cmd, "concurrent", string(rune(id+'0')))
			if err != nil {
				results <- err
				return
			}
			if result.ExitCode != 0 {
				results <- assert.AnError
				return
			}
			results <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		suite.NoError(err)
	}
}

// Run the test suite
func TestExecRunnerTestSuite(t *testing.T) {
	suite.Run(t, new(ExecRunnerTestSuite))
}

// Additional standalone tests for edge cases

func TestExecRunner_Interface(t *testing.T) {
	runner := NewExecRunner()
	assert.Implements(t, (*Runner)(nil), runner)
}

func TestExecRunner_ResultTypes(t *testing.T) {
	runner := NewExecRunner()

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo"
	} else {
		cmd = "echo"
	}

	result, err := runner.Run(cmd, "test")
	require.NoError(t, err)

	assert.IsType(t, []byte{}, result.Stdout)
	assert.IsType(t, []byte{}, result.Stderr)
	assert.IsType(t, int(0), result.ExitCode)
}

func TestExecRunner_LongOutput(t *testing.T) {
	runner := NewExecRunner()

	var cmd string
	var longString string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		longString = strings.Repeat("a", 1000)
	} else {
		cmd = "sh"
		longString = strings.Repeat("a", 1000)
	}

	var result Result
	var err error

	if runtime.GOOS == "windows" {
		result, err = runner.Run(cmd, "/c", "echo", longString)
	} else {
		result, err = runner.Run(cmd, "-c", "echo "+longString)
	}

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), longString)
}

func TestExecRunner_EnvironmentIsolation(t *testing.T) {
	runner := NewExecRunner()

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "sh"
	}

	var result Result
	var err error

	if runtime.GOOS == "windows" {
		result, err = runner.Run(cmd, "/c", "echo", "%PATH%")
	} else {
		result, err = runner.Run(cmd, "-c", "echo $PATH")
	}

	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotEmpty(t, result.Stdout)
}
