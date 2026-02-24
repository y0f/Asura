package checker

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/y0f/Asura/internal/safenet"
	"github.com/y0f/Asura/internal/storage"
)

type MQTTChecker struct {
	AllowPrivate bool
}

func (c *MQTTChecker) Type() string { return "mqtt" }

func (c *MQTTChecker) Check(ctx context.Context, monitor *storage.Monitor) (*Result, error) {
	var settings storage.MQTTSettings
	if len(monitor.Settings) > 0 {
		if err := json.Unmarshal(monitor.Settings, &settings); err != nil {
			return &Result{Status: "down", Message: fmt.Sprintf("invalid settings: %v", err)}, nil
		}
	}

	target := monitor.Target
	if _, _, err := net.SplitHostPort(target); err != nil {
		if settings.UseTLS {
			target = target + ":8883"
		} else {
			target = target + ":1883"
		}
	}

	clientID := settings.ClientID
	if clientID == "" {
		clientID = fmt.Sprintf("asura-%d", rand.Int63())
	}

	timeout := time.Duration(monitor.Timeout) * time.Second
	baseDial := (&net.Dialer{Timeout: timeout, Control: safenet.MaybeDialControl(c.AllowPrivate)}).DialContext

	dialFn := baseDial
	if socks := ProxyDialer(monitor.ProxyURL, baseDial); socks != nil {
		dialFn = socks
	}

	start := time.Now()
	conn, err := dialFn(ctx, "tcp", target)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("MQTT connection failed: %v", err),
		}, nil
	}
	defer conn.Close()

	if settings.UseTLS {
		host, _, _ := net.SplitHostPort(target)
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			elapsed = time.Since(start).Milliseconds()
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      fmt.Sprintf("MQTT TLS handshake failed: %v", err),
			}, nil
		}
		conn = tlsConn
	}

	conn.SetDeadline(time.Now().Add(timeout))

	connectPkt := buildConnectPacket(clientID, settings.Username, settings.Password)
	if _, err := conn.Write(connectPkt); err != nil {
		elapsed = time.Since(start).Milliseconds()
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("MQTT CONNECT send failed: %v", err),
		}, nil
	}

	connackBuf := make([]byte, 4)
	if _, err := readFull(conn, connackBuf); err != nil {
		elapsed = time.Since(start).Milliseconds()
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("MQTT CONNACK read failed: %v", err),
		}, nil
	}

	if connackBuf[0]>>4 != 2 { // CONNACK packet type
		elapsed = time.Since(start).Milliseconds()
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("MQTT unexpected packet type: %d", connackBuf[0]>>4),
		}, nil
	}

	returnCode := connackBuf[3]
	if returnCode != 0 {
		elapsed = time.Since(start).Milliseconds()
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("MQTT CONNACK rejected: code=%d (%s)", returnCode, mqttConnackError(returnCode)),
		}, nil
	}

	if settings.Topic != "" {
		subPkt := buildSubscribePacket(1, settings.Topic)
		if _, err := conn.Write(subPkt); err != nil {
			elapsed = time.Since(start).Milliseconds()
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      fmt.Sprintf("MQTT SUBSCRIBE send failed: %v", err),
			}, nil
		}

		subackBuf := make([]byte, 5)
		if _, err := readFull(conn, subackBuf); err != nil {
			elapsed = time.Since(start).Milliseconds()
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      fmt.Sprintf("MQTT SUBACK read failed: %v", err),
			}, nil
		}

		if subackBuf[0]>>4 != 9 { // SUBACK packet type
			elapsed = time.Since(start).Milliseconds()
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      fmt.Sprintf("MQTT unexpected packet type: %d (expected SUBACK)", subackBuf[0]>>4),
			}, nil
		}

		if subackBuf[4] == 0x80 {
			elapsed = time.Since(start).Milliseconds()
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      "MQTT subscription rejected",
			}, nil
		}
	}

	// Send DISCONNECT
	conn.Write([]byte{0xe0, 0x00})

	elapsed = time.Since(start).Milliseconds()

	return &Result{
		Status:       "up",
		ResponseTime: elapsed,
		Message:      "MQTT connection successful",
	}, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func encodeMQTTString(s string) []byte {
	b := make([]byte, 2+len(s))
	binary.BigEndian.PutUint16(b[0:2], uint16(len(s)))
	copy(b[2:], s)
	return b
}

func encodeRemainingLength(length int) []byte {
	var buf []byte
	for {
		digit := byte(length % 128)
		length /= 128
		if length > 0 {
			digit |= 0x80
		}
		buf = append(buf, digit)
		if length == 0 {
			break
		}
	}
	return buf
}

func buildConnectPacket(clientID, username, password string) []byte {
	var payload []byte
	payload = append(payload, encodeMQTTString("MQTT")...)
	payload = append(payload, 4) // protocol level 4 (MQTT 3.1.1)

	var connectFlags byte
	connectFlags |= 0x02 // clean session

	if username != "" {
		connectFlags |= 0x80
	}
	if password != "" {
		connectFlags |= 0x40
	}

	payload = append(payload, connectFlags)
	payload = append(payload, 0, 30) // keepalive 30 seconds
	payload = append(payload, encodeMQTTString(clientID)...)

	if username != "" {
		payload = append(payload, encodeMQTTString(username)...)
	}
	if password != "" {
		payload = append(payload, encodeMQTTString(password)...)
	}

	var pkt []byte
	pkt = append(pkt, 0x10) // CONNECT packet type
	pkt = append(pkt, encodeRemainingLength(len(payload))...)
	pkt = append(pkt, payload...)
	return pkt
}

func buildSubscribePacket(packetID uint16, topic string) []byte {
	var payload []byte
	// packet identifier
	payload = append(payload, byte(packetID>>8), byte(packetID))
	payload = append(payload, encodeMQTTString(topic)...)
	payload = append(payload, 0) // QoS 0

	var pkt []byte
	pkt = append(pkt, 0x82) // SUBSCRIBE packet type with QoS 1
	pkt = append(pkt, encodeRemainingLength(len(payload))...)
	pkt = append(pkt, payload...)
	return pkt
}

func mqttConnackError(code byte) string {
	switch code {
	case 1:
		return "unacceptable protocol version"
	case 2:
		return "identifier rejected"
	case 3:
		return "server unavailable"
	case 4:
		return "bad username or password"
	case 5:
		return "not authorized"
	default:
		return "unknown"
	}
}

func decodeRemainingLength(data []byte) (int, int) {
	multiplier := 1
	value := 0
	for i, b := range data {
		value += int(b&0x7f) * multiplier
		multiplier *= 128
		if b&0x80 == 0 {
			return value, i + 1
		}
		if i >= 3 {
			break
		}
	}
	return value, len(data)
}

