package util

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

func GetURIHostPort(uri string, defaultPort int) (string, int, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", 0, err
	}
	host := u.Host
	if host == "" {
		return "", 0, nil
	}
	h, p, err := net.SplitHostPort(host)
	if err == nil {
		port, err := strconv.Atoi(p)
		return h, port, err
	}
	return strings.Trim(host, "[]"), defaultPort, nil
}
