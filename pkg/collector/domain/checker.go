package domain

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zijiren233/sealos-state-metric/pkg/util"
)

// IPHealth represents the health status of a specific IP for a domain
type IPHealth struct {
	Domain string
	IP     string // Specific IP address

	// HTTP check
	HTTPOk       bool
	HTTPError    string
	ResponseTime time.Duration

	// Certificate check
	CertOk     bool
	CertError  string
	CertExpiry time.Duration

	LastChecked time.Time
}

// DomainChecker performs health checks on domains
type DomainChecker struct {
	timeout   time.Duration
	checkHTTP bool
	checkDNS  bool
	checkCert bool
}

// NewDomainChecker creates a new domain checker
func NewDomainChecker(timeout time.Duration, checkHTTP, checkDNS, checkCert bool) *DomainChecker {
	return &DomainChecker{
		timeout:   timeout,
		checkHTTP: checkHTTP,
		checkDNS:  checkDNS,
		checkCert: checkCert,
	}
}

// CheckIPs performs all enabled checks on a domain for each of its IPs
func (dc *DomainChecker) CheckIPs(
	ctx context.Context,
	domain string,
	logger *log.Entry,
) []*IPHealth {
	now := time.Now()

	// First, get the IPs for the domain
	var ips []string
	if dc.checkDNS || dc.checkHTTP {
		dnsResult := util.CheckDNS(ctx, domain, dc.timeout)
		if !dnsResult.Success {
			logger.WithFields(log.Fields{
				"domain": domain,
				"error":  dnsResult.Error,
			}).Warn("DNS resolution failed")

			return nil
		}

		ips = dnsResult.IPs
	}

	// Get certificate info (shared across all IPs)
	var (
		certInfo *util.CertInfo
		certErr  error
	)

	if dc.checkCert {
		certInfo, certErr = util.GetTLSCert(domain, dc.timeout)
	}

	// Check each IP individually
	results := make([]*IPHealth, 0, len(ips))
	for _, ip := range ips {
		health := &IPHealth{
			Domain:      domain,
			IP:          ip,
			LastChecked: now,
		}

		// HTTP check for this specific IP
		if dc.checkHTTP {
			result := util.CheckHTTPWithIP(ctx, domain, ip, dc.timeout)
			health.HTTPOk = result.Success
			health.HTTPError = result.Error
			health.ResponseTime = result.ResponseTime

			logger.WithFields(log.Fields{
				"domain":       domain,
				"ip":           ip,
				"success":      health.HTTPOk,
				"responseTime": health.ResponseTime,
			}).Debug("HTTP check completed")
		}

		// Certificate check (same for all IPs)
		if dc.checkCert {
			if certErr != nil {
				health.CertOk = false
				health.CertError = certErr.Error()
			} else {
				health.CertOk = certInfo.IsValid

				health.CertExpiry = certInfo.ExpiresIn
				if !certInfo.IsValid {
					health.CertError = "certificate expired or not yet valid"
				}
			}

			logger.WithFields(log.Fields{
				"domain":    domain,
				"ip":        ip,
				"success":   health.CertOk,
				"expiresIn": health.CertExpiry,
			}).Debug("Certificate check completed")
		}

		results = append(results, health)
	}

	return results
}
