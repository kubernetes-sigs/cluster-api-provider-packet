package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"time"
)

// GenerateClient generate a client key and cert, signed by the CA
func GenerateClient(cn string, caCert *x509.Certificate, caKey *rsa.PrivateKey) ([]byte, *rsa.PrivateKey, error) {
	// simple RSA key
	privKey, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate RSA private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)

	notBefore := time.Now()
	notAfter := notBefore.Add(oneYear)

	subject := pkix.Name{
		Organization: []string{"system:masters"},
		CommonName:   cn,
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      subject,
		NotBefore:    notBefore,
		NotAfter:     notAfter,

		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client certificate: %v", err)
	}
	return derBytes, privKey, nil
}
