package util

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// CertInfo contains parsed certificate information
type CertInfo struct {
	CommonName string
	Issuer     string
	NotBefore  time.Time
	NotAfter   time.Time
	ExpiresIn  time.Duration
	IsValid    bool
	Error      string
}

// ParseCertificate parses a PEM-encoded certificate
func ParseCertificate(certPEM []byte) (*CertInfo, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	now := time.Now()
	expiresIn := cert.NotAfter.Sub(now)
	isValid := now.After(cert.NotBefore) && now.Before(cert.NotAfter)

	return &CertInfo{
		CommonName: cert.Subject.CommonName,
		Issuer:     cert.Issuer.CommonName,
		NotBefore:  cert.NotBefore,
		NotAfter:   cert.NotAfter,
		ExpiresIn:  expiresIn,
		IsValid:    isValid,
	}, nil
}

// ParseCertificateSafe safely parses a certificate and returns error info if it fails
func ParseCertificateSafe(certPEM []byte) *CertInfo {
	info, err := ParseCertificate(certPEM)
	if err != nil {
		return &CertInfo{
			IsValid: false,
			Error:   err.Error(),
		}
	}

	return info
}
