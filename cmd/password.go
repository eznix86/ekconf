package cmd

import (
	"github.com/eznix86/ekconf/internal/password"
)

func passwordResolve(passwordFlag string, passwordStdin bool, useKeychain bool) (string, error) {
	return password.Resolve(passwordFlag, passwordStdin, useKeychain)
}
