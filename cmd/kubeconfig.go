package cmd

import (
	"fmt"
	"os"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func readDecryptedConfigData(password []byte) ([]byte, error) {
	encPath, err := config.EncPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(encPath)
	if err != nil {
		return nil, fmt.Errorf("read config.enc: %w", err)
	}

	plaintext, err := crypto.Open(data, password)
	if err != nil {
		return nil, fmt.Errorf("decrypt (wrong password?): %w", err)
	}

	return plaintext, nil
}

func loadDecryptedKubeconfig(password []byte) (*clientcmdapi.Config, error) {
	plaintext, err := readDecryptedConfigData(password)
	if err != nil {
		return nil, err
	}

	kubeconfig, err := clientcmd.Load(plaintext)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	return kubeconfig, nil
}
