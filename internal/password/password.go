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

const minPasswordLength = 12

var (
	lookupCurrentUser = user.Current
	openTTY           = os.OpenFile
)

func Resolve(ctx context.Context, passwordFlag string, passwordStdin, useKeychain bool, envPassword string) ([]byte, error) {
	if passwordFlag != "" {
		return []byte(passwordFlag), nil
	}

	if passwordStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return []byte(strings.TrimRight(string(data), "\r\n")), nil
	}

	if envPassword != "" {
		return []byte(envPassword), nil
	}

	if useKeychain {
		pw, err := keyring.Get(keyringService, currentUser())
		if err == nil {
			return []byte(pw), nil
		}
	}

	return promptPassword(ctx, useKeychain)
}

func Store(password []byte) error {
	return keyring.Set(keyringService, currentUser(), string(password))
}

func Delete() error {
	return keyring.Delete(keyringService, currentUser())
}

func promptPassword(ctx context.Context, useKeychain bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tty, err := openTTY("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		if useKeychain {
			return nil, fmt.Errorf(noTTYPasswordMessage + ", or keychain")
		}
		return nil, errors.New(noTTYPasswordMessage)
	}
	defer tty.Close()

	fmt.Fprint(os.Stderr, "Password: ")
	bytePW, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("read password: %w", err)
	}

	return bytePW, nil
}

func PromptNewPassword(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tty, err := openTTY("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return nil, errors.New(noTTYPasswordMessage)
	}
	defer tty.Close()

	fmt.Fprint(os.Stderr, "New password: ")
	pw, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("read new password: %w", err)
	}

	if err := validateNewPassword(pw); err != nil {
		clear(pw)
		return nil, err
	}

	fmt.Fprint(os.Stderr, "Confirm new password: ")
	confirm, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		clear(pw)
		return nil, fmt.Errorf("read confirm: %w", err)
	}

	if !bytes.Equal(pw, confirm) {
		clear(pw)
		clear(confirm)
		return nil, fmt.Errorf("passwords do not match")
	}

	clear(confirm)
	return pw, nil
}

func validateNewPassword(pw []byte) error {
	if len(pw) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	return nil
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
