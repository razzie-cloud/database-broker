package postgres

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
	InstanceName string    `db:"instance_name,primary"`
	Host         string    `db:"-"`
	Port         int       `db:"-"`
	Database     string    `db:"db_name"`
	Username     string    `db:"db_user"`
	Password     string    `db:"db_password"`
	CreatedAt    time.Time `db:"created_at"`
}

func (Instance) Table() string { return "instances" }

func (i Instance) GetURI() string {
	return buildConnURI(i.Host, i.Port, i.Database, i.Username, i.Password)
}

func (i Instance) GetJSON() any {
	return InstanceResponse{
		Instance:  i.InstanceName,
		Host:      i.Host,
		Port:      i.Port,
		Database:  i.Database,
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
	Database  string    `json:"database"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	URI       string    `json:"uri"`
	CreatedAt time.Time `json:"created_at"`
}

func buildConnURI(host string, port int, db, user, pass string) string {
	userpass := url.UserPassword(user, pass).String()
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	return fmt.Sprintf("postgres://%s@%s/%s?sslmode=prefer", userpass, addr, db)
}
