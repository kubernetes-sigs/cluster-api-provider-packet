package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"sigs.k8s.io/cluster-api/pkg/cert"
)

// GenerateSelfSigned generate a self-signed key and cert
func GenerateSelfSigned(cn, hosts string) ([]byte, *rsa.PrivateKey, error) {
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
		Organization: []string{"Packet"},
	}
	if cn != "" {
		subject.CommonName = cn
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      subject,
		NotBefore:    notBefore,
		NotAfter:     notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
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
		return nil, nil, fmt.Errorf("failed to create CA certificate: %v", err)
	}
	return derBytes, privKey, nil
}

// GenerateSelfSignedCertAndKey generate a self-signed cert and key and then place it in the appropriate format for sigs.k8s.io/cluster-api/pkg/cert.CertificateAuthority
func GenerateSelfSignedCertAndKey(cn, hosts string) (*cert.CertificateAuthority, error) {
	certificate, key, err := GenerateSelfSigned(cn, hosts)
	if err != nil {
		return nil, err
	}
	ca := cert.CertificateAuthority{
		Certificate: PemEncodeCert(certificate),
		PrivateKey:  PemEncodeKey(key),
	}
	return &ca, nil
}
