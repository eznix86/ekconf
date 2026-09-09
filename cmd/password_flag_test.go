package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWarnAboutPasswordFlag(t *testing.T) {
	t.Run("warns when the flag is used", func(t *testing.T) {
		resetCommandTestState(t)
		passwordFlag = "hunter2-hunter2"

		var buf bytes.Buffer
		warnAboutPasswordFlag(&buf)

		assert.Contains(t, buf.String(), "--password puts the password in the process table")
		assert.Contains(t, buf.String(), "--password-stdin")
	})

	t.Run("silent otherwise", func(t *testing.T) {
		resetCommandTestState(t)
		passwordFlag = ""

		var buf bytes.Buffer
		warnAboutPasswordFlag(&buf)

		assert.Empty(t, buf.String())
	})

	t.Run("never prints the password itself", func(t *testing.T) {
		resetCommandTestState(t)
		passwordFlag = "correct-horse-battery-staple"

		var buf bytes.Buffer
		warnAboutPasswordFlag(&buf)

		assert.NotContains(t, buf.String(), "correct-horse-battery-staple")
	})
}

// Documented examples must not teach the unsafe flag.
func TestExamplesDoNotAdvertiseThePasswordFlag(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		assert.NotContains(t, c.Example, "--password=", "%s example should use --password-stdin", c.Name())
	}
}
