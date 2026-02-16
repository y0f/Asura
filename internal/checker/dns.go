package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/asura-monitor/asura/internal/storage"
)

type DNSChecker struct{}

func (c *DNSChecker) Type() string { return "dns" }

func (c *DNSChecker) Check(ctx context.Context, monitor *storage.Monitor) (*Result, error) {
	var settings storage.DNSSettings
	if len(monitor.Settings) > 0 {
		json.Unmarshal(monitor.Settings, &settings)
	}

	recordType := settings.RecordType
	if recordType == "" {
		recordType = "A"
	}

	resolver := net.DefaultResolver
	if settings.Server != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Duration(monitor.Timeout) * time.Second}
				return d.DialContext(ctx, "udp", settings.Server+":53")
			},
		}
	}

	start := time.Now()
	var records []string
	var err error

	switch recordType {
	case "A":
		var addrs []net.IP
		addrs, err = resolver.LookupIP(ctx, "ip4", monitor.Target)
		for _, a := range addrs {
			records = append(records, a.String())
		}
	case "AAAA":
		var addrs []net.IP
		addrs, err = resolver.LookupIP(ctx, "ip6", monitor.Target)
		for _, a := range addrs {
			records = append(records, a.String())
		}
	case "CNAME":
		var cname string
		cname, err = resolver.LookupCNAME(ctx, monitor.Target)
		if cname != "" {
			records = append(records, cname)
		}
	case "MX":
		var mxs []*net.MX
		mxs, err = resolver.LookupMX(ctx, monitor.Target)
		for _, mx := range mxs {
			records = append(records, fmt.Sprintf("%d %s", mx.Pref, mx.Host))
		}
	case "TXT":
		records, err = resolver.LookupTXT(ctx, monitor.Target)
	case "NS":
		var nss []*net.NS
		nss, err = resolver.LookupNS(ctx, monitor.Target)
		for _, ns := range nss {
			records = append(records, ns.Host)
		}
	default:
		return &Result{
			Status:  "down",
			Message: fmt.Sprintf("unsupported record type: %s", recordType),
		}, nil
	}

	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("DNS lookup failed: %v", err),
		}, nil
	}

	if len(records) == 0 {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      "no records found",
		}, nil
	}

	return &Result{
		Status:       "up",
		ResponseTime: elapsed,
		DNSRecords:   records,
		Message:      fmt.Sprintf("found %d record(s)", len(records)),
	}, nil
}
