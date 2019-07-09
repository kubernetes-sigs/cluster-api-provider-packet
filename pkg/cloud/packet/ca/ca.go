package ca

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"sigs.k8s.io/cluster-api/pkg/cert"
)

const (
	rsaBits = 2048
	oneYear = 365 * 24 * time.Hour
)

// Generate a key and cert
func Generate(cn, hosts string) ([]byte, *rsa.PrivateKey, error) {
	if hosts == "" && cn == "" {
		return nil, nil, fmt.Errorf("must specify at least one hostname/IP or CN")
	}
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
		Organization: []string{"Zededa"},
	}
	if cn != "" {
		subject.CommonName = cn
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      subject,
		NotBefore:    notBefore,
		NotAfter:     notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	hostnames := strings.Split(hosts, ",")
	for _, h := range hostnames {
		if h == "" {
			continue
		}
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %v", err)
	}
	return derBytes, privKey, nil
}

// GenerateCertAndKey generate a cert and key and then place it in the appropriate format for sigs.k8s.io/cluster-api/pkg/cert.CertificateAuthority
func GenerateCertAndKey(cn, hosts string) (*cert.CertificateAuthority, error) {
	certificate, key, err := Generate(cn, hosts)
	if err != nil {
		return nil, err
	}
	ca := cert.CertificateAuthority{
		Certificate: PemEncodeCert(certificate),
		PrivateKey:  PemEncodeKey(key),
	}
	return &ca, nil
}

// PemEncodeCert take certificate DER bytes and PEM encode them
func PemEncodeCert(cert []byte) []byte {
	out := &bytes.Buffer{}
	pem.Encode(out, &pem.Block{Type: "CERTIFICATE", Bytes: cert})
	b := make([]byte, out.Len())
	copy(b, out.Bytes())
	return b
}

// PemEncodeKey take an RSA private key and PEM encode it
func PemEncodeKey(key *rsa.PrivateKey) []byte {
	out := &bytes.Buffer{}
	pem.Encode(out, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	b := make([]byte, out.Len())
	copy(b, out.Bytes())
	return b
}
