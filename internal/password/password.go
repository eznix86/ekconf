package password

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"golang.org/x/term"

	"github.com/zalando/go-keyring"
)

const keyringService = "ekconf"

const noTTYPasswordMessage = "not a terminal and no password provided via --password, --password-stdin, or EKCONF_PASSWORD"

var (
	lookupCurrentUser = user.Current
	openTTY           = os.OpenFile
)

func Resolve(ctx context.Context, passwordFlag string, passwordStdin, useKeychain bool, envPassword string) (string, error) {
	if passwordFlag != "" {
		return passwordFlag, nil
	}

	if passwordStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}

	if envPassword != "" {
		return envPassword, nil
	}

	if useKeychain {
		pw, err := keyring.Get(keyringService, currentUser())
		if err == nil {
			return pw, nil
		}
	}

	return promptPassword(ctx, useKeychain)
}

func Store(password string) error {
	return keyring.Set(keyringService, currentUser(), password)
}

func Delete() error {
	return keyring.Delete(keyringService, currentUser())
}

func promptPassword(ctx context.Context, useKeychain bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	tty, err := openTTY("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		if useKeychain {
			return "", fmt.Errorf(noTTYPasswordMessage + ", or keychain")
		}
		return "", errors.New(noTTYPasswordMessage)
	}
	defer tty.Close()

	fmt.Fprint(os.Stderr, "Password: ")
	bytePW, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	defer clear(bytePW)

	return string(bytePW), nil
}

func PromptNewPassword(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	tty, err := openTTY("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return "", errors.New(noTTYPasswordMessage)
	}
	defer tty.Close()

	fmt.Fprint(os.Stderr, "New password: ")
	pw, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read new password: %w", err)
	}
	defer clear(pw)

	fmt.Fprint(os.Stderr, "Confirm new password: ")
	confirm, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read confirm: %w", err)
	}
	defer clear(confirm)

	if !bytes.Equal(pw, confirm) {
		return "", fmt.Errorf("passwords do not match")
	}

	return string(pw), nil
}

func currentUser() string {
	if u, err := lookupCurrentUser(); err == nil {
		if u.Username != "" {
			return u.Username
		}
		if u.Uid != "" {
			return "uid-" + u.Uid
		}
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "uid-unknown"
}
