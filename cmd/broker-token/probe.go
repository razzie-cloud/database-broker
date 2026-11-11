package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/razzie-cloud/database-broker/internal/auth"
)

// probe only checks denied routes and never performs credential provisioning.
func probe(args []string) error {
	var err error
	for attempt := range 5 {
		err = probeOnce(args)
		if err == nil {
			return nil
		}
		if attempt < 4 {
			time.Sleep(time.Duration(200*(1<<attempt)) * time.Millisecond)
		}
	}
	return err
}
func probeOnce(args []string) error {
	bad := errors.New("broker enforcement probe failed")
	f := flag.NewFlagSet("probe", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	origin := f.String("origin", "", "broker HTTPS origin")
	caFile := f.String("ca-file", "", "CA file")
	addresses := f.String("addresses", "", "comma separated replica IP:port addresses")
	tokenFile := f.String("token-file", "", "optional scoped token file")
	client := f.String("client", "", "client identifier")
	readyAddresses := f.String("ready-addresses", "", "comma separated application IP:port addresses")
	readinessOnly := f.Bool("readiness-only", false, "only check readiness during staged migration")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return bad
	}
	u, e := url.Parse(*origin)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return bad
	}
	ca, e := os.ReadFile(*caFile)
	if e != nil || len(ca) > 1<<20 {
		return bad
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return bad
	}
	token := ""
	if *tokenFile != "" {
		b, e := os.ReadFile(*tokenFile)
		if e != nil || len(b) > 128 {
			return bad
		}
		token = string(b)
		if _, e := auth.ParseToken(token); e != nil || !auth.ValidClient(*client) {
			return bad
		}
	}
	targets := strings.Split(*addresses, ",")
	if *addresses == "" {
		return bad
	}
	for _, address := range targets {
		host, _, e := net.SplitHostPort(address)
		if e != nil || net.ParseIP(host) == nil {
			return bad
		}
		transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, address)
		}}
		c := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		tests := []struct {
			path   string
			status int
		}{{"/readyz", 200}, {"/v1/instances/valkey", 401}, {"/v1/instances/valkey/__broker_probe__/uri", 401}}
		if token != "" {
			q := url.Values{"client": {*client}, "token": {token}}.Encode()
			tests = append(tests, struct {
				path   string
				status int
			}{"/v1/instances/__broker_probe__/__broker_probe__/uri?" + q, 403}, struct {
				path   string
				status int
			}{"/v1/instances/valkey?" + q, 403})
		}
		if *readinessOnly {
			tests = tests[:1]
		}
		for _, test := range tests {
			resp, e := c.Get(*origin + test.path)
			if e != nil {
				transport.CloseIdleConnections()
				return bad
			}
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			if resp.StatusCode != test.status || (test.status != 200 && resp.Header.Get("Cache-Control") != "no-store") {
				transport.CloseIdleConnections()
				return bad
			}
		}
		transport.CloseIdleConnections()
	}
	if *readyAddresses != "" {
		c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		defer c.CloseIdleConnections()
		for _, address := range strings.Split(*readyAddresses, ",") {
			host, _, e := net.SplitHostPort(address)
			if e != nil || net.ParseIP(host) == nil {
				return bad
			}
			resp, e := c.Get("http://" + address + "/readyz")
			if e != nil {
				return bad
			}
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			if resp.StatusCode != 200 {
				return bad
			}
		}
	}
	return nil
}
