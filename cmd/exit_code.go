package cmd

import "fmt"

type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("exit status %d", e.code)
}

func (e *exitCodeError) ExitCode() int {
	return e.code
}

func usageErrorf(format string, args ...any) error {
	return &exitCodeError{code: 2, msg: fmt.Sprintf(format, args...)}
}
