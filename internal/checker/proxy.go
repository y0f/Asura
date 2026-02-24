package checker

import (
	"context"
	"net"
	"net/url"

	"golang.org/x/net/proxy"
)

// ProxyDialer returns a DialContext function that routes through the given proxy URL.
// For SOCKS5 proxies, it uses golang.org/x/net/proxy.
// For HTTP proxies, it returns nil (callers should use http.Transport.Proxy instead).
// If proxyURL is empty, it returns nil.
func ProxyDialer(proxyURL string, baseDial func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if proxyURL == "" {
		return nil
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}

	if u.Scheme != "socks5" {
		return nil
	}

	var auth *proxy.Auth
	if u.User != nil {
		auth = &proxy.Auth{User: u.User.Username()}
		if p, ok := u.User.Password(); ok {
			auth.Password = p
		}
	}

	dialer, err := proxy.SOCKS5("tcp", u.Host, auth, &contextDialer{dial: baseDial})
	if err != nil {
		return nil
	}

	if cd, ok := dialer.(proxy.ContextDialer); ok {
		return cd.DialContext
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}
}

// HTTPProxyURL returns a *url.URL for use with http.Transport.Proxy if the
// proxy URL uses HTTP. Returns nil for SOCKS5 or empty URLs.
func HTTPProxyURL(proxyURL string) *url.URL {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		return u
	}
	return nil
}

// contextDialer wraps a DialContext func as a proxy.Dialer.
type contextDialer struct {
	dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (d *contextDialer) Dial(network, addr string) (net.Conn, error) {
	return d.dial(context.Background(), network, addr)
}
