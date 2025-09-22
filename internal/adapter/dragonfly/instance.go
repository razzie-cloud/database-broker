package dragonfly

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
)

var _ adapter.Instance = (*Instance)(nil)

type Instance struct {
	Instance  string    `json:"-"`
	Host      string    `json:"-"`
	Port      int       `json:"-"`
	Namespace string    `json:"namespace"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	URI       string    `json:"uri"`
	CreatedAt time.Time `json:"created_at"`
}

func (i Instance) GetURI() string {
	return buildConnURI(i.Host, i.Port, i.Username, i.Password)
}

func (i Instance) GetJSON() any {
	return InstanceResponse{
		Instance:  i.Instance,
		Host:      i.Host,
		Port:      i.Port,
		Namespace: i.Namespace,
		Username:  i.Username,
		Password:  i.Password,
		URI:       i.GetURI(),
		CreatedAt: i.CreatedAt,
	}
}

type InstanceResponse struct {
	Instance  string    `json:"instance"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Namespace string    `json:"namespace"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	URI       string    `json:"uri"`
	CreatedAt time.Time `json:"created_at"`
}

func buildConnURI(host string, port int, user, pass string) string {
	userpass := url.UserPassword(user, pass).String()
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	return fmt.Sprintf("redis://%s@%s/0", userpass, addr)
}
