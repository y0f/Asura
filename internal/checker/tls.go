package checker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/y0f/Asura/internal/safenet"
	"github.com/y0f/Asura/internal/storage"
)

type TLSChecker struct {
	AllowPrivate bool
}

func (c *TLSChecker) Type() string { return "tls" }

func (c *TLSChecker) Check(ctx context.Context, monitor *storage.Monitor) (*Result, error) {
	var settings storage.TLSSettings
	if len(monitor.Settings) > 0 {
		json.Unmarshal(monitor.Settings, &settings)
	}

	warnDays := settings.WarnDaysBefore
	if warnDays <= 0 {
		warnDays = 30
	}

	target := monitor.Target
	// Add default port if missing
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = target + ":443"
	}

	host, _, _ := net.SplitHostPort(target)

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: time.Duration(monitor.Timeout) * time.Second,
			Control: safenet.MaybeDialControl(c.AllowPrivate),
		},
		Config: &tls.Config{
			ServerName: host,
		},
	}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("TLS connection failed: %v", err),
		}, nil
	}
	defer conn.Close()

	tlsConn := conn.(*tls.Conn)
	state := tlsConn.ConnectionState()

	if len(state.PeerCertificates) == 0 {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      "no certificates presented",
		}, nil
	}

	cert := state.PeerCertificates[0]
	expiry := cert.NotAfter
	expiryUnix := expiry.Unix()
	daysUntilExpiry := int(time.Until(expiry).Hours() / 24)

	result := &Result{
		Status:       "up",
		ResponseTime: elapsed,
		CertExpiry:   &expiryUnix,
		Message:      fmt.Sprintf("cert expires in %d days (%s)", daysUntilExpiry, expiry.Format("2006-01-02")),
	}

	if daysUntilExpiry <= 0 {
		result.Status = "down"
		result.Message = fmt.Sprintf("certificate expired on %s", expiry.Format("2006-01-02"))
	} else if daysUntilExpiry <= warnDays {
		result.Status = "degraded"
		result.Message = fmt.Sprintf("cert expires in %d days (warning threshold: %d)", daysUntilExpiry, warnDays)
	}

	return result, nil
}
