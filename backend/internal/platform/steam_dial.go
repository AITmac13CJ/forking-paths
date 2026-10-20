package platform

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

func newSteamHTTP() *http.Client {
	dialer := &net.Dialer{Timeout: 6 * time.Second}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         dialer.DialContext,
		DialTLSContext:      steamDialTLS(dialer),
		ForceAttemptHTTP2:   false,
		TLSHandshakeTimeout: 6 * time.Second,
		IdleConnTimeout:     30 * time.Second,
	}
	return &http.Client{Timeout: 40 * time.Second, Transport: transport}
}

func steamDialTLS(base *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if host != "api.steampowered.com" && host != "steamcommunity.com" {
			return dialTLS(ctx, base, network, addr, host)
		}
		ips := steamIPs(ctx, host)
		if len(ips) == 0 {
			return dialTLS(ctx, base, network, addr, host)
		}
		return raceTLS(ctx, base, host, ips)
	}
}

func dialTLS(ctx context.Context, base *net.Dialer, network, addr, host string) (net.Conn, error) {
	conn, err := base.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return tlsConn, nil
}

func steamIPs(ctx context.Context, host string) []net.IP {
	servers := []string{"", "119.29.29.29", "223.5.5.5", "114.114.114.114"}
	lctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var mu sync.Mutex
	var all []net.IP
	var wg sync.WaitGroup
	for _, server := range servers {
		wg.Add(1)
		go func(server string) {
			defer wg.Done()
			ips := lookupSteam(lctx, server, host)
			mu.Lock()
			all = append(all, ips...)
			mu.Unlock()
		}(server)
	}
	wg.Wait()
	seen := map[string]bool{}
	out := make([]net.IP, 0, len(all))
	for _, ip := range all {
		ip = ip.To4()
		if ip == nil {
			continue
		}
		key := ip.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ip)
		if len(out) == 16 {
			break
		}
	}
	return out
}

func lookupSteam(ctx context.Context, server, host string) []net.IP {
	resolver := net.DefaultResolver
	if server != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 1500 * time.Millisecond}
				return d.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
			},
		}
	}
	ips, err := resolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil
	}
	return ips
}

func raceTLS(ctx context.Context, base *net.Dialer, host string, ips []net.IP) (net.Conn, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, len(ips))
	for _, ip := range ips {
		go func(ip net.IP) {
			dctx, dcancel := context.WithTimeout(ctx, 5*time.Second)
			defer dcancel()
			raw, err := base.DialContext(dctx, "tcp4", net.JoinHostPort(ip.String(), "443"))
			if err != nil {
				ch <- result{err: err}
				return
			}
			conn := tls.Client(raw, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
			if err := conn.HandshakeContext(dctx); err != nil {
				raw.Close()
				ch <- result{err: err}
				return
			}
			ch <- result{conn: conn}
		}(ip)
	}
	var first error
	for i := 0; i < len(ips); i++ {
		got := <-ch
		if got.err != nil {
			if first == nil {
				first = got.err
			}
			continue
		}
		cancel()
		go func(rest int) {
			for rest > 0 {
				extra := <-ch
				if extra.conn != nil {
					extra.conn.Close()
				}
				rest--
			}
		}(len(ips) - i - 1)
		return got.conn, nil
	}
	if first == nil {
		first = context.DeadlineExceeded
	}
	return nil, first
}
