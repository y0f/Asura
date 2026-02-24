package checker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/y0f/Asura/internal/safenet"
	"github.com/y0f/Asura/internal/storage"
	"golang.org/x/net/http2"
)

type GRPCChecker struct {
	AllowPrivate bool
}

func (c *GRPCChecker) Type() string { return "grpc" }

func (c *GRPCChecker) Check(ctx context.Context, monitor *storage.Monitor) (*Result, error) {
	var settings storage.GRPCSettings
	if len(monitor.Settings) > 0 {
		if err := json.Unmarshal(monitor.Settings, &settings); err != nil {
			return &Result{Status: "down", Message: fmt.Sprintf("invalid settings: %v", err)}, nil
		}
	}

	target := monitor.Target
	if _, _, err := net.SplitHostPort(target); err != nil {
		if settings.UseTLS {
			target = target + ":443"
		} else {
			target = target + ":50051"
		}
	}

	timeout := time.Duration(monitor.Timeout) * time.Second
	baseDial := (&net.Dialer{
		Timeout: timeout,
		Control: safenet.MaybeDialControl(c.AllowPrivate),
	}).DialContext

	dialFn := baseDial
	if socks := ProxyDialer(monitor.ProxyURL, baseDial); socks != nil {
		dialFn = socks
	}

	reqBody := encodeGRPCFrame(encodeHealthRequest(settings.ServiceName))

	var scheme string
	var transport http.RoundTripper

	if settings.UseTLS {
		scheme = "https"
		host, _, _ := net.SplitHostPort(target)
		transport = &http2.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         host,
				InsecureSkipVerify: settings.SkipTLSVerify,
			},
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				rawConn, err := dialFn(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				tlsConn := tls.Client(rawConn, cfg)
				if err := tlsConn.HandshakeContext(ctx); err != nil {
					rawConn.Close()
					return nil, err
				}
				return tlsConn, nil
			},
		}
	} else {
		scheme = "http"
		transport = &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return dialFn(ctx, network, addr)
			},
		}
	}

	url := fmt.Sprintf("%s://%s/grpc.health.v1.Health/Check", scheme, target)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return &Result{Status: "down", Message: fmt.Sprintf("invalid request: %v", err)}, nil
	}
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("TE", "trailers")

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("gRPC request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	grpcStatus := resp.Trailer.Get("grpc-status")
	if grpcStatus == "" {
		grpcStatus = resp.Header.Get("grpc-status")
	}

	if grpcStatus != "" && grpcStatus != "0" {
		grpcMsg := resp.Trailer.Get("grpc-message")
		if grpcMsg == "" {
			grpcMsg = resp.Header.Get("grpc-message")
		}
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			StatusCode:   resp.StatusCode,
			Message:      fmt.Sprintf("gRPC error: status=%s message=%s", grpcStatus, grpcMsg),
		}, nil
	}

	payload, err := decodeGRPCFrame(body)
	if err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			StatusCode:   resp.StatusCode,
			Message:      fmt.Sprintf("invalid gRPC frame: %v", err),
		}, nil
	}

	healthStatus := decodeHealthResponse(payload)

	result := &Result{
		ResponseTime: elapsed,
		StatusCode:   resp.StatusCode,
	}

	switch healthStatus {
	case 1: // SERVING
		result.Status = "up"
		result.Message = "gRPC health: SERVING"
	case 2: // NOT_SERVING
		result.Status = "down"
		result.Message = "gRPC health: NOT_SERVING"
	case 3: // SERVICE_UNKNOWN
		result.Status = "down"
		result.Message = "gRPC health: SERVICE_UNKNOWN"
	default:
		result.Status = "down"
		result.Message = fmt.Sprintf("gRPC health: UNKNOWN(%d)", healthStatus)
	}

	return result, nil
}

func encodeGRPCFrame(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = 0 // no compression
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

func decodeGRPCFrame(data []byte) ([]byte, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("frame too short: %d bytes", len(data))
	}
	length := binary.BigEndian.Uint32(data[1:5])
	if uint32(len(data)-5) < length {
		return nil, fmt.Errorf("frame truncated: want %d, have %d", length, len(data)-5)
	}
	return data[5 : 5+length], nil
}

func encodeHealthRequest(service string) []byte {
	if service == "" {
		return nil
	}
	serviceBytes := []byte(service)
	buf := make([]byte, 0, 2+len(serviceBytes))
	buf = append(buf, 0x0a) // field 1, wire type 2 (length-delimited)
	buf = append(buf, byte(len(serviceBytes)))
	buf = append(buf, serviceBytes...)
	return buf
}

func decodeHealthResponse(data []byte) int32 {
	if len(data) == 0 {
		return 1 // empty response = SERVING
	}
	for i := 0; i < len(data); {
		if i >= len(data) {
			break
		}
		tag := data[i]
		fieldNum := tag >> 3
		wireType := tag & 0x07
		i++
		if fieldNum == 1 && wireType == 0 {
			if i < len(data) {
				return int32(data[i])
			}
		}
		// skip unknown fields
		switch wireType {
		case 0: // varint
			for i < len(data) && data[i]&0x80 != 0 {
				i++
			}
			i++
		case 2: // length-delimited
			if i < len(data) {
				l := int(data[i])
				i += 1 + l
			}
		default:
			return 0
		}
	}
	return 0
}
