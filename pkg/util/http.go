package util

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// HTTPCheckResult contains the result of an HTTP check
type HTTPCheckResult struct {
	Success      bool
	ResponseTime time.Duration
	StatusCode   int
	Error        string
}

// CheckHTTP performs an HTTP/HTTPS health check
func CheckHTTP(ctx context.Context, url string, timeout time.Duration) *HTTPCheckResult {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS12,
			},
		},
	}

	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &HTTPCheckResult{
			Success: false,
			Error:   fmt.Sprintf("failed to create request: %v", err),
		}
	}

	resp, err := client.Do(req)
	responseTime := time.Since(start)

	if err != nil {
		return &HTTPCheckResult{
			Success:      false,
			ResponseTime: responseTime,
			Error:        fmt.Sprintf("request failed: %v", err),
		}
	}

	defer resp.Body.Close()

	return &HTTPCheckResult{
		Success:      resp.StatusCode >= 200 && resp.StatusCode < 500,
		ResponseTime: responseTime,
		StatusCode:   resp.StatusCode,
	}
}

// CheckHTTPWithIP performs an HTTP/HTTPS health check to a specific IP address
func CheckHTTPWithIP(
	ctx context.Context,
	domain, ip string,
	timeout time.Duration,
) *HTTPCheckResult {
	// Create a transport that dials the specific IP
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Override the address with our specific IP
				return (&net.Dialer{
					Timeout: 15 * time.Second,
				}).DialContext(ctx, network, net.JoinHostPort(ip, "443"))
			},
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS12,
				ServerName:         domain, // Important: use domain for SNI
			},
		},
	}

	start := time.Now()

	// Build URL with domain (not IP)
	url := "https://" + domain + "/"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &HTTPCheckResult{
			Success: false,
			Error:   fmt.Sprintf("failed to create request: %v", err),
		}
	}

	// Set Host header to domain
	req.Host = domain

	resp, err := client.Do(req)
	responseTime := time.Since(start)

	if err != nil {
		return &HTTPCheckResult{
			Success:      false,
			ResponseTime: responseTime,
			Error:        fmt.Sprintf("request failed: %v", err),
		}
	}

	defer resp.Body.Close()

	return &HTTPCheckResult{
		Success:      resp.StatusCode >= 200 && resp.StatusCode < 500,
		ResponseTime: responseTime,
		StatusCode:   resp.StatusCode,
	}
}

// GetTLSCert retrieves the TLS certificate from a domain
func GetTLSCert(domain string, timeout time.Duration) (*CertInfo, error) {
	dialer := &tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", domain+":443")
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, errors.New("not a TLS connection")
	}

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, errors.New("no certificates found")
	}

	cert := state.PeerCertificates[0]
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
