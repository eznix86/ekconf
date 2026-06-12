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
	cmd.SetEnvPassword(os.Getenv("EKCONF_PASSWORD"))
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
