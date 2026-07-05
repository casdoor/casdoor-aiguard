// Copyright 2025 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/casdoor/casdoor-aiguard/conf"
)

const (
	caCertFileName = "aiguard-ca.crt"
	caKeyFileName  = "aiguard-ca.key"
)

// CertificateAuthority is aiguard's local root CA used to terminate (MITM)
// intercepted TLS connections so their plaintext HTTP payload can be
// recognized. Its certificate is meant to be exported from the Web UI and
// trusted on the host (or baked into the container base image) that runs
// the agents being governed.
type CertificateAuthority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey

	leafMutex sync.Mutex
	leafCache map[string]*tls.Certificate
}

// LoadOrCreateCA loads the CA from conf.GetCaCertDir(), generating a fresh
// one on first run.
func LoadOrCreateCA() (*CertificateAuthority, error) {
	dir := conf.GetCaCertDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	certPath := filepath.Join(dir, caCertFileName)
	keyPath := filepath.Join(dir, caKeyFileName)

	if _, err := os.Stat(certPath); err == nil {
		return loadCA(certPath, keyPath)
	}

	return generateCA(certPath, keyPath)
}

func generateCA(certPath, keyPath string) (*CertificateAuthority, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Casdoor AIGuard Local CA",
			Organization: []string{"Casdoor AIGuard"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	if err := writePem(certPath, "CERTIFICATE", der); err != nil {
		return nil, err
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := writePem(keyPath, "EC PRIVATE KEY", keyBytes); err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	return &CertificateAuthority{cert: cert, key: key, leafCache: map[string]*tls.Certificate{}}, nil
}

func loadCA(certPath, keyPath string) (*CertificateAuthority, error) {
	certPem, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPem, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	certBlock, _ := pem.Decode(certPem)
	if certBlock == nil {
		return nil, fmt.Errorf("invalid CA certificate at %s", certPath)
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode(keyPem)
	if keyBlock == nil {
		return nil, fmt.Errorf("invalid CA key at %s", keyPath)
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	return &CertificateAuthority{cert: cert, key: key, leafCache: map[string]*tls.Certificate{}}, nil
}

func writePem(path, blockType string, der []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: der})
}

// CertPEMPath returns the path to the CA certificate, for the Web UI download endpoint.
func (ca *CertificateAuthority) CertPEMPath() string {
	return filepath.Join(conf.GetCaCertDir(), caCertFileName)
}

// CACertPath returns the path aiguard's CA certificate is (or will be)
// stored at, without requiring a loaded CertificateAuthority instance.
// Used by the Web UI's CA-download endpoint.
func CACertPath() string {
	return filepath.Join(conf.GetCaCertDir(), caCertFileName)
}

// LeafCertificateFor returns (generating and caching on first use) a
// server certificate for host, signed by this CA, used to terminate the TLS
// connection an intercepted agent believes it made directly to host.
func (ca *CertificateAuthority) LeafCertificateFor(host string) (*tls.Certificate, error) {
	ca.leafMutex.Lock()
	defer ca.leafMutex.Unlock()

	if cert, ok := ca.leafCache[host]; ok {
		return cert, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{der, ca.cert.Raw},
		PrivateKey:  key,
	}
	ca.leafCache[host] = tlsCert
	return tlsCert, nil
}
