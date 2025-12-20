package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manager handles CA certificate operations
type Manager struct {
	caCert     *x509.Certificate
	caKey      *rsa.PrivateKey
	certCache  *CertCache
	cacheDir   string
}

// NewManager creates a new CA manager
func NewManager(caCertPath, caKeyPath, cacheDir string) (*Manager, error) {
	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(caCertPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create CA cert directory: %w", err)
	}
	if cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cert cache directory: %w", err)
		}
	}

	m := &Manager{
		cacheDir: cacheDir,
	}

	// Try to load existing CA
	if err := m.loadCA(caCertPath, caKeyPath); err != nil {
		// Generate new CA if doesn't exist
		if os.IsNotExist(err) {
			if err := m.generateCA(caCertPath, caKeyPath); err != nil {
				return nil, fmt.Errorf("failed to generate CA: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to load CA: %w", err)
		}
	}

	// Initialize certificate cache
	m.certCache = NewCertCache(1000) // Cache up to 1000 certs

	return m, nil
}

// loadCA loads the CA certificate and key from files
func (m *Manager) loadCA(certPath, keyPath string) error {
	// Load certificate
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Load private key
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("failed to parse key PEM")
	}

	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		// Try PKCS8
		keyInterface, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		key, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("private key is not RSA")
		}
	}

	m.caCert = cert
	m.caKey = key

	return nil
}

// generateCA generates a new CA certificate and key
func (m *Manager) generateCA(certPath, keyPath string) error {
	// Generate private key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{"AI Proxy CA"},
			OrganizationalUnit: []string{"Development"},
			CommonName:         "AI Proxy Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Parse the certificate for internal use
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("failed to parse created certificate: %w", err)
	}

	// Write certificate to file
	certFile, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	// Write key to file (with restrictive permissions)
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	keyDER := x509.MarshalPKCS1PrivateKey(key)
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	m.caCert = cert
	m.caKey = key

	return nil
}

// GetCertificate returns a TLS certificate for the given host
func (m *Manager) GetCertificate(host string) (*tls.Certificate, error) {
	// Strip port for cache key
	hostname := host
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			hostname = h
		}
	}

	// Check cache first
	if cert := m.certCache.Get(hostname); cert != nil {
		return cert, nil
	}

	// Generate new certificate
	cert, err := m.generateHostCert(host)
	if err != nil {
		return nil, err
	}

	// Cache it
	m.certCache.Put(hostname, cert)

	return cert, nil
}

// generateHostCert generates a certificate for a specific host
func (m *Manager) generateHostCert(host string) (*tls.Certificate, error) {
	// Strip port if present (e.g., "example.com:443" -> "example.com")
	hostname := host
	if strings.Contains(host, ":") {
		var err error
		hostname, _, err = net.SplitHostPort(host)
		if err != nil {
			// If SplitHostPort fails, it might be an IPv6 address without port
			hostname = host
		}
	}

	// Generate private key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"AI Proxy"},
			CommonName:   hostname,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0), // 1 year
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{hostname},
	}

	// Sign with CA
	certDER, err := x509.CreateCertificate(rand.Reader, &template, m.caCert, &key.PublicKey, m.caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Create TLS certificate
	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER, m.caCert.Raw},
		PrivateKey:  key,
	}

	return tlsCert, nil
}

// CACertPEM returns the CA certificate in PEM format
func (m *Manager) CACertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: m.caCert.Raw,
	})
}

// CACert returns the CA certificate
func (m *Manager) CACert() *x509.Certificate {
	return m.caCert
}

// TLSConfig returns a TLS config that uses this CA for dynamic cert generation
func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return m.GetCertificate(hello.ServerName)
		},
	}
}
