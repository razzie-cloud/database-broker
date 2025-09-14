package postgres

import (
	"fmt"
	"net/url"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
)

var _ adapter.Instance = (*Instance)(nil)

type Instance struct {
	InstanceName string    `db:"instance_name,primary"`
	DBHost       string    `db:"-"`
	DBPort       int       `db:"-"`
	DBName       string    `db:"db_name"`
	DBUser       string    `db:"db_user"`
	DBPassword   string    `db:"db_password"`
	CreatedAt    time.Time `db:"created_at"`
}

func (Instance) Table() string { return "instances" }

func (i Instance) GetURI() string {
	return buildConnURI(i.DBHost, i.DBPort, i.DBName, i.DBUser, i.DBPassword)
}

func (i Instance) GetJSON() any {
	return InstanceResponse{
		Instance:  i.InstanceName,
		Host:      i.DBHost,
		Port:      i.DBPort,
		DBName:    i.DBName,
		DBUser:    i.DBUser,
		DBPass:    i.DBPassword,
		URI:       i.GetURI(),
		CreatedAt: i.CreatedAt,
	}
}

type InstanceResponse struct {
	Instance  string    `json:"instance"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	DBName    string    `json:"db_name"`
	DBUser    string    `json:"db_user"`
	DBPass    string    `json:"db_password"`
	URI       string    `json:"uri"`
	CreatedAt time.Time `json:"created_at"`
}

func buildConnURI(host string, port int, db, user, pass string) string {
	escUser := url.UserPassword(user, pass).String()
	return fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=prefer", escUser, host, port, db)
}
