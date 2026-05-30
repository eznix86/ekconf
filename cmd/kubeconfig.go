package cmd

import (
	"fmt"
	"os"

	"github.com/eznix86/ekconf/internal/config"
	"github.com/eznix86/ekconf/internal/crypto"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func readDecryptedConfigData(password string) ([]byte, error) {
	encPath, err := config.EncPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(encPath)
	if err != nil {
		return nil, fmt.Errorf("read config.enc: %w", err)
	}

	ef, err := crypto.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parse encrypted file: %w", err)
	}

	plaintext, err := crypto.Decrypt(ef, password)
	if err != nil {
		return nil, fmt.Errorf("decrypt (wrong password?): %w", err)
	}

	return plaintext, nil
}

func loadDecryptedKubeconfig(password string) (*clientcmdapi.Config, error) {
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
