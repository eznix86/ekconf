package password

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/zalando/go-keyring"
)

const keyringService = "ekconf"

func Resolve(passwordFlag string, passwordStdin bool, useKeychain bool) (string, error) {
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

	if env := os.Getenv("EKCONF_PASSWORD"); env != "" {
		return env, nil
	}

	if useKeychain {
		pw, err := keyring.Get(keyringService, currentUser())
		if err == nil {
			return pw, nil
		}
	}

	return promptPassword()
}

func Store(password string) error {
	return keyring.Set(keyringService, currentUser(), password)
}

func Delete() error {
	return keyring.Delete(keyringService, currentUser())
}

func promptPassword() (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return "", fmt.Errorf("not a terminal and no password provided via --password, --password-stdin, EKCONF_PASSWORD, or keychain")
	}
	defer tty.Close()

	fmt.Fprint(os.Stderr, "Password: ")
	bytePW, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(bytePW), nil
}

func PromptNewPassword() (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return "", fmt.Errorf("not a terminal and no password provided via --password, --password-stdin, EKCONF_PASSWORD, or keychain")
	}
	defer tty.Close()

	fmt.Fprint(os.Stderr, "New password: ")
	pw, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read new password: %w", err)
	}

	fmt.Fprint(os.Stderr, "Confirm new password: ")
	confirm, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read confirm: %w", err)
	}

	if string(pw) != string(confirm) {
		return "", fmt.Errorf("passwords do not match")
	}

	return string(pw), nil
}

func currentUser() string {
	u := os.Getenv("USER")
	if u == "" {
		u = os.Getenv("USERNAME")
	}
	if u == "" {
		u = "unknown"
	}
	return u
}
