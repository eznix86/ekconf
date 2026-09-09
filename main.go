package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/eznix86/ekconf/cmd"
)

func main() {
	cmd.SetEnvPassword(os.Getenv(cmd.PasswordEnvVar))
	if err := os.Unsetenv(cmd.PasswordEnvVar); err != nil {
		fmt.Fprintln(os.Stderr, "unset "+cmd.PasswordEnvVar+":", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := cmd.ExecuteContext(ctx); err != nil {
		stop()
		fmt.Fprintln(os.Stderr, err)
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}

	stop()
}
