package util

import (
	"net/url"
	"strconv"
	"strings"
)

func GetURIHostPort(uri string, defaultPort int) (string, int, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", 0, err
	}
	if strings.Contains(u.Host, ":") {
		parts := strings.Split(u.Host, ":")
		host := parts[0]
		port, err := strconv.Atoi(parts[1])
		return host, port, err
	}
	return u.Host, defaultPort, nil
}
