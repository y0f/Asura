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
	"net/url"
	"strconv"
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
		if err := json.Unmarshal(monitor.Settings, &settings); err != nil {
			return &Result{Status: "down", Message: fmt.Sprintf("invalid settings: %v", err)}, nil
		}
	}

	method := settings.Method
	if method == "" {
		method = "GET"
	}

	target := monitor.Target
	if settings.CacheBuster {
		target = cacheBustURL(target)
	}

	var bodyReader io.Reader
	if settings.Body != "" {
		bodyReader = strings.NewReader(settings.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, bodyReader)
	if err != nil {
		return &Result{Status: "down", Message: fmt.Sprintf("invalid request: %v", err)}, nil
	}

	applyBodyAndHeaders(req, settings)
	applyAuthentication(req, settings)

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

	maxRedirects := resolveMaxRedirects(settings)

	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(monitor.Timeout) * time.Second,
	}
	if maxRedirects == 0 {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		limit := maxRedirects
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= limit {
				return fmt.Errorf("stopped after %d redirects", limit)
			}
			return nil
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		if ue, ok := err.(*url.Error); ok && ue.Err == http.ErrUseLastResponse {
			// not actually an error — we just don't follow redirects
		} else {
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      fmt.Sprintf("request failed: %v", err),
			}, nil
		}
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

	status := "up"
	var msg string
	if settings.ExpectedStatus > 0 && resp.StatusCode != settings.ExpectedStatus {
		status = "down"
		msg = fmt.Sprintf("expected status %d, got %d", settings.ExpectedStatus, resp.StatusCode)
	}

	result := &Result{
		Status:       status,
		ResponseTime: elapsed,
		StatusCode:   resp.StatusCode,
		Message:      msg,
		Body:         body,
		BodyHash:     bodyHash,
		Headers:      headers,
	}

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		expiry := resp.TLS.PeerCertificates[0].NotAfter.Unix()
		result.CertExpiry = &expiry
	}

	return result, nil
}

func cacheBustURL(target string) string {
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	return target + sep + "_=" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func applyBodyAndHeaders(req *http.Request, settings storage.HTTPSettings) {
	if settings.Body != "" {
		switch settings.BodyEncoding {
		case "xml":
			req.Header.Set("Content-Type", "application/xml")
		case "form":
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		case "json":
			req.Header.Set("Content-Type", "application/json")
		}
	}
	for k, v := range settings.Headers {
		req.Header.Set(k, v)
	}
}

func applyAuthentication(req *http.Request, settings storage.HTTPSettings) {
	switch settings.AuthMethod {
	case "basic":
		if settings.BasicAuthUser != "" {
			req.SetBasicAuth(settings.BasicAuthUser, settings.BasicAuthPass)
		}
	case "bearer":
		if settings.BearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+settings.BearerToken)
		}
	default:
		if settings.BasicAuthUser != "" {
			req.SetBasicAuth(settings.BasicAuthUser, settings.BasicAuthPass)
		}
	}
}

func resolveMaxRedirects(s storage.HTTPSettings) int {
	if s.MaxRedirects > 0 {
		return s.MaxRedirects
	}
	if s.FollowRedirects != nil {
		if !*s.FollowRedirects {
			return 0
		}
		return 10
	}
	return 10
}
