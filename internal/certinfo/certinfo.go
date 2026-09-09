package certinfo

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"time"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const certificateBlockType = "CERTIFICATE"

func Expiry(authInfo *clientcmdapi.AuthInfo) (time.Time, bool) {
	if authInfo == nil {
		return time.Time{}, false
	}

	pemData := authInfo.ClientCertificateData
	if len(pemData) == 0 && authInfo.ClientCertificate != "" {
		data, err := os.ReadFile(authInfo.ClientCertificate)
		if err != nil {
			return time.Time{}, false
		}
		pemData = data
	}

	return earliestNotAfter(pemData)
}

func earliestNotAfter(pemData []byte) (time.Time, bool) {
	var earliest time.Time

	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != certificateBlockType {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if earliest.IsZero() || cert.NotAfter.Before(earliest) {
			earliest = cert.NotAfter
		}
	}

	if earliest.IsZero() {
		return time.Time{}, false
	}
	return earliest, true
}
