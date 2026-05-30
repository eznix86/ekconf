package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eznix86/ekconf/cmd"
)

func main() {
	cmd.SetEnvPassword(os.Getenv("EKCONF_PASSWORD"))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) == 1 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		cmd.MaybePrintUpdateNotice(ctx, os.Stderr)
	}
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}
