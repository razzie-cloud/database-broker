package valkey

import (
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
)

var _ adapter.Instance = (*Instance)(nil)

type Instance struct {
	Instance  string    `json:"instance"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Database  int       `json:"database"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
	TLS       bool      `json:"-"`
}

func (i Instance) GetURI() string {
	scheme := "redis"
	if i.TLS {
		scheme = "rediss"
	}
	u := url.URL{
		Scheme: scheme,
		User:   url.UserPassword(i.Username, i.Password),
		Host:   net.JoinHostPort(i.Host, strconv.Itoa(i.Port)),
		Path:   "/" + strconv.Itoa(i.Database),
	}
	return u.String()
}

func (i Instance) GetJSON() any {
	return struct {
		Instance
		URI string `json:"uri"`
	}{i, i.GetURI()}
}
