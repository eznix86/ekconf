package cmd

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A long-running child such as `kubectl port-forward` keeps the decrypted
// kubeconfig on disk for its whole lifetime, so every signal that routinely
// ends one has to wipe the file first. SIGHUP arrives when the terminal
// closes, SIGQUIT when the user sends Ctrl-\.
func TestCleanupSignals_CoverTheWaysATerminalEndsAChild(t *testing.T) {
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT} {
		assert.Contains(t, cleanupSignals, sig, "%s must wipe the temp kubeconfig", sig)
	}
}
