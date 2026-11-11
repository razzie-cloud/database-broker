package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"io"
)

// Public PKI validation never receives the TLS private key or token master.
func validateTLS(args []string, input io.Reader) error {
	bad := errors.New("invalid public broker PKI")
	f := flag.NewFlagSet("validate-tls", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	hostname := f.String("hostname", "", "certificate DNS name")
	if f.Parse(args) != nil || f.NArg() != 0 || *hostname == "" {
		return bad
	}
	b, e := io.ReadAll(io.LimitReader(input, 2<<20+1))
	if e != nil || len(b) > 2<<20 {
		return bad
	}
	var p struct {
		CA   string `json:"ca"`
		Cert string `json:"cert"`
	}
	if json.Unmarshal(b, &p) != nil {
		return bad
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(p.CA)) {
		return bad
	}
	rest := bytes.TrimSpace([]byte(p.Cert))
	var chain []*x509.Certificate
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" {
			return bad
		}
		cert, e := x509.ParseCertificate(block.Bytes)
		if e != nil {
			return bad
		}
		chain = append(chain, cert)
		rest = bytes.TrimSpace(next)
	}
	if len(chain) == 0 {
		return bad
	}
	intermediates := x509.NewCertPool()
	for _, cert := range chain[1:] {
		intermediates.AddCert(cert)
	}
	if _, e := chain[0].Verify(x509.VerifyOptions{DNSName: *hostname, Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); e != nil {
		return bad
	}
	return nil
}
