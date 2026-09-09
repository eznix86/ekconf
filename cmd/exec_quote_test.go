package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The command name stays unquoted so the interactive shell still resolves
// aliases and functions, which is the reason shell mode exists. Every argument
// after it is quoted, so a value carrying shell metacharacters is passed
// through as data instead of being re-parsed as syntax.
func TestShellCommandLine(t *testing.T) {
	tests := map[string]struct {
		args []string
		want string
	}{
		"plain command":        {[]string{"kubectl", "get", "pods"}, "kubectl 'get' 'pods'"},
		"command name is bare": {[]string{"helm"}, "helm"},
		"argument with spaces": {[]string{"kubectl", "get", "pod name"}, "kubectl 'get' 'pod name'"},
		"command chaining":     {[]string{"echo", "x; touch /tmp/pwned"}, `echo 'x; touch /tmp/pwned'`},
		"substitution":         {[]string{"echo", "$(id)"}, `echo '$(id)'`},
		"backticks":            {[]string{"echo", "`id`"}, "echo '`id`'"},
		"pipe":                 {[]string{"echo", "a | tee /tmp/x"}, `echo 'a | tee /tmp/x'`},
		"embedded quote":       {[]string{"echo", "it's"}, `echo 'it'\''s'`},
		"empty argument":       {[]string{"echo", ""}, "echo ''"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, shellCommandLine(tc.args))
		})
	}
}
