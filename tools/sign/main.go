// Command sign produces the ed25519 signature over a release artifact that
// ekconf update verifies before installing.
package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

const keyEnvVar = "EKCONF_RELEASE_SIGNING_KEY"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sign:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: sign <artifact> <signature-output>")
	}

	key, err := signingKey(os.Getenv(keyEnvVar))
	if err != nil {
		return err
	}

	//nolint:gosec // The paths are goreleaser's own artifact list, not user input.
	artifact, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}

	//nolint:gosec // The paths are goreleaser's own artifact list, not user input.
	return os.WriteFile(args[1], ed25519.Sign(key, artifact), 0o600)
}

func signingKey(pemData string) (ed25519.PrivateKey, error) {
	if pemData == "" {
		return nil, fmt.Errorf("%s is not set", keyEnvVar)
	}

	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("%s is not a PEM block", keyEnvVar)
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse signing key: %w", err)
	}

	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("signing key is %T, want ed25519.PrivateKey", parsed)
	}

	return key, nil
}
