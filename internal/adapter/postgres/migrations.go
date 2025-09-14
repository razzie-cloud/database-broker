package postgres

import (
	"context"

	"github.com/go-rel/migration"
	"github.com/go-rel/rel"
)

func MigrateCreateInstances(schema *rel.Schema) {
	schema.CreateTable("instances", func(t *rel.Table) {
		t.Text("instance_name")
		t.PrimaryKey("instance_name")
		t.Text("db_name")
		t.Text("db_user")
		t.Text("db_password")
		t.DateTime("created_at", rel.Default("NOW()"))
	})
}

func RollbackCreateInstances(schema *rel.Schema) {
	schema.DropTable("instances")
}

func migrate(repo rel.Repository) {
	m := migration.New(repo)
	m.Register(1, MigrateCreateInstances, RollbackCreateInstances)
	m.Migrate(context.Background())
}
