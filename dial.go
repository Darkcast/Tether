package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

const dialTimeout = 15 * time.Second

// dialTarget opens a TCP connection to target (host:port), routing through
// proxyStr when set. Falls back to HTTPS_PROXY / ALL_PROXY env vars when
// proxyStr is empty. Supported schemes: direct (empty), http://, https://, socks5://.
func dialTarget(target, proxyStr string) (net.Conn, error) {
	if proxyStr == "" {
		proxyStr = proxyFromEnv()
	}
	if proxyStr == "" {
		return net.DialTimeout("tcp", target, dialTimeout)
	}
	u, err := url.Parse(proxyStr)
	if err != nil {
		return nil, fmt.Errorf("proxy URL: %w", err)
	}
	switch u.Scheme {
	case "http":
		return dialHTTPProxy(target, u, false)
	case "https":
		return dialHTTPProxy(target, u, true)
	case "socks5":
		return dialSOCKS5(target, u)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
	}
}

func proxyFromEnv() string {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func dialHTTPProxy(target string, u *url.URL, outerTLS bool) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", u.Host, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial proxy %s: %w", u.Host, err)
	}
	if outerTLS {
		tc := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), InsecureSkipVerify: true})
		if err := tc.Handshake(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("proxy TLS handshake: %w", err)
		}
		conn = tc
	}

	hdr := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if u.User != nil {
		usr := u.User.Username()
		pwd, _ := u.User.Password()
		creds := base64.StdEncoding.EncodeToString([]byte(usr + ":" + pwd))
		hdr += "Proxy-Authorization: Basic " + creds + "\r\n"
	}
	hdr += "\r\n"
	if _, err := io.WriteString(conn, hdr); err != nil {
		conn.Close()
		return nil, fmt.Errorf("CONNECT write: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("CONNECT response: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("CONNECT: proxy returned %s", resp.Status)
	}
	return conn, nil
}

// dialSOCKS5 performs a hand-rolled RFC 1928 / RFC 1929 SOCKS5 CONNECT.
func dialSOCKS5(target string, u *url.URL) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", u.Host, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial socks5 %s: %w", u.Host, err)
	}

	authMethod := byte(0x00)
	if u.User != nil {
		authMethod = 0x02
	}

	// Greeting: VER(1) NMETHODS(1) METHODS(n)
	if _, err := conn.Write([]byte{0x05, 0x01, authMethod}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 greeting: %w", err)
	}
	meth := make([]byte, 2)
	if _, err := io.ReadFull(conn, meth); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 server choice: %w", err)
	}
	if meth[0] != 0x05 || meth[1] == 0xFF {
		conn.Close()
		return nil, fmt.Errorf("socks5: no acceptable auth method")
	}

	// User/password sub-negotiation (RFC 1929)
	if meth[1] == 0x02 {
		usr := u.User.Username()
		pwd, _ := u.User.Password()
		msg := []byte{0x01, byte(len(usr))}
		msg = append(msg, []byte(usr)...)
		msg = append(msg, byte(len(pwd)))
		msg = append(msg, []byte(pwd)...)
		if _, err := conn.Write(msg); err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks5 auth send: %w", err)
		}
		ar := make([]byte, 2)
		if _, err := io.ReadFull(conn, ar); err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks5 auth response: %w", err)
		}
		if ar[1] != 0x00 {
			conn.Close()
			return nil, fmt.Errorf("socks5 auth failed")
		}
	}

	// CONNECT request
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 target: %w", err)
	}
	portNum, err := net.LookupPort("tcp", portStr)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 port: %w", err)
	}

	var req []byte
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append([]byte{0x05, 0x01, 0x00, 0x01}, ip4...)
		} else {
			req = append([]byte{0x05, 0x01, 0x00, 0x04}, ip.To16()...)
		}
	} else {
		req = append([]byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}, []byte(host)...)
	}
	req = append(req, byte(portNum>>8), byte(portNum))

	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 connect: %w", err)
	}

	// Reply: VER REP RSV ATYP [BND.ADDR] BND.PORT
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 reply: %w", err)
	}
	if hdr[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 connect refused (code %d)", hdr[1])
	}

	// Drain bound address and port from reply
	var addrLen int
	switch hdr[3] {
	case 0x01:
		addrLen = 4
	case 0x03:
		lb := make([]byte, 1)
		if _, err := io.ReadFull(conn, lb); err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks5 reply domain len: %w", err)
		}
		addrLen = int(lb[0])
	case 0x04:
		addrLen = 16
	}
	drain := make([]byte, addrLen+2)
	if _, err := io.ReadFull(conn, drain); err != nil {
		conn.Close()
		return nil, fmt.Errorf("socks5 reply drain: %w", err)
	}

	return conn, nil
}
