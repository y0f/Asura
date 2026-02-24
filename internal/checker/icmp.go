package checker

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/y0f/asura/internal/safenet"
	"github.com/y0f/asura/internal/storage"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type ICMPChecker struct {
	AllowPrivate bool
}

func (c *ICMPChecker) Type() string { return "icmp" }

func (c *ICMPChecker) Check(ctx context.Context, monitor *storage.Monitor) (*Result, error) {
	timeout := time.Duration(monitor.Timeout) * time.Second
	start := time.Now()

	addrs, err := net.DefaultResolver.LookupIP(ctx, "ip4", monitor.Target)
	if err != nil || len(addrs) == 0 {
		return &Result{
			Status:       "down",
			ResponseTime: time.Since(start).Milliseconds(),
			Message:      fmt.Sprintf("DNS resolution failed: %v", err),
		}, nil
	}
	dst := addrs[0]

	if !c.AllowPrivate && safenet.IsPrivateIP(dst) {
		return &Result{
			Status:  "down",
			Message: fmt.Sprintf("blocked: connections to private/reserved IP %s are not allowed", dst),
		}, nil
	}

	conn, err := listenICMP()
	if err != nil {
		return &Result{Status: "down", Message: fmt.Sprintf("ICMP listen failed: %v", err)}, nil
	}
	defer conn.Close()

	if err := sendEchoRequest(conn, dst); err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: time.Since(start).Milliseconds(),
			Message:      fmt.Sprintf("send failed: %v", err),
		}, nil
	}

	return readEchoReply(conn, dst, start, timeout)
}

func listenICMP() (*icmp.PacketConn, error) {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
	}
	return conn, err
}

func sendEchoRequest(conn *icmp.PacketConn, dst net.IP) error {
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1,
			Data: []byte("asura-ping"),
		},
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return err
	}

	var dstAddr net.Addr
	if conn.LocalAddr().Network() == "udp4" {
		dstAddr = &net.UDPAddr{IP: dst}
	} else {
		dstAddr = &net.IPAddr{IP: dst}
	}
	_, err = conn.WriteTo(wb, dstAddr)
	return err
}

func readEchoReply(conn *icmp.PacketConn, dst net.IP, start time.Time, timeout time.Duration) (*Result, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	rb := make([]byte, 1500)
	n, _, err := conn.ReadFrom(rb)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:       "down",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("receive failed: %v", err),
		}, nil
	}

	proto := 1
	if conn.LocalAddr().Network() == "udp4" {
		proto = 58
	}
	rm, err := icmp.ParseMessage(proto, rb[:n])
	if err != nil {
		rm, err = icmp.ParseMessage(1, rb[:n])
		if err != nil {
			return &Result{
				Status:       "down",
				ResponseTime: elapsed,
				Message:      fmt.Sprintf("parse reply failed: %v", err),
			}, nil
		}
	}

	if rm.Type == ipv4.ICMPTypeEchoReply {
		return &Result{
			Status:       "up",
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("ping %s: %dms", dst, elapsed),
		}, nil
	}

	return &Result{
		Status:       "down",
		ResponseTime: elapsed,
		Message:      fmt.Sprintf("unexpected ICMP type: %v", rm.Type),
	}, nil
}
