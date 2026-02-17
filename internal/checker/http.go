package checker

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/y0f/Asura/internal/safenet"
	"github.com/y0f/Asura/internal/storage"
)

const maxBodyRead = 1 << 20 // 1MB

type HTTPChecker struct {
	AllowPrivate bool
}

func (c *HTTPChecker) Type() string { return "http" }

func (c *HTTPChecker) Check(ctx context.Context, monitor *storage.Monitor) (*Result, error) {
	var settings storage.HTTPSettings
	if len(monitor.Settings) > 0 {
		json.Unmarshal(monitor.Settings, &settings)
	}

	method := settings.Method
	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if settings.Body != "" {
		bodyReader = strings.NewReader(settings.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, monitor.Target, bodyReader)
	if err != nil {
		return &Result{Status: "down", Message: fmt.Sprintf("invalid request: %v", err)}, nil
	}

	for k, v := range settings.Headers {
		req.Header.Set(k, v)
	}

	if settings.BasicAuthUser != "" {
		req.SetBasicAuth(settings.BasicAuthUser, settings.BasicAuthPass)
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: time.Duration(monitor.Timeout) * time.Second,
			Control: safenet.MaybeDialControl(c.AllowPrivate),
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: settings.SkipTLSVerify,
		},
		DisableKeepAlives: true,
	}

	followRedirects := true
	if settings.FollowRedirects != nil {
		followRedirects = *settings.FollowRedirects
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(monitor.Timeout) * time.Second,
	}
	if !followRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyRead))
	body := string(bodyBytes)

	h := sha256.Sum256(bodyBytes)
	bodyHash := hex.EncodeToString(h[:])

	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	result := &Result{
		Status:       "up",
		ResponseTime: elapsed,
		StatusCode:   resp.StatusCode,
		Body:         body,
		BodyHash:     bodyHash,
		Headers:      headers,
	}

	// Check TLS cert expiry if available
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		expiry := resp.TLS.PeerCertificates[0].NotAfter.Unix()
		result.CertExpiry = &expiry
	}

	return result, nil
}
