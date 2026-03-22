package spec

import (
	"testing"
)

func TestMigrationsMultiSchema(t *testing.T) {
	cli, teardown := NewTestCLI(t)
	defer teardown()

	dbTeardown := cli.RandomDatabase()
	defer dbTeardown()

	if !cli.IsNamespaceSupported() {
		t.Skip("driver does not support namespace")
		return
	}

	cli.Files(
		// init set — creates the schema in public context
		"set_init.fyml",
		`
apiVersion: migrations/v1
kind: MigrationSet
metadata:
  name: init
spec:
  namespace:
    schema: public
  migrations:
    - create_app_schema
        `,
		"01_create_app_schema.fyml",
		`
apiVersion: migrations/v1
kind: Migration
metadata:
  name: create_app_schema
spec:
  version: "v1"
  run:
    up:
      sql: CREATE SCHEMA app;
    down:
      sql: DROP SCHEMA app CASCADE;
`,
		// app set — uses the app schema
		"set_app.fyml",
		`
apiVersion: migrations/v1
kind: MigrationSet
metadata:
  name: app
spec:
  namespace:
    schema: app
  migrations:
    - create_users
        `,
		"02_create_users.fyml",
		`
apiVersion: migrations/v1
kind: Migration
metadata:
  name: create_users
spec:
  version: "v1"
  run:
    up:
      sql: |
        CREATE TABLE users(id integer, name varchar(255));

        CREATE INDEX idx_users_name ON users(name);
    down:
      sql: |
        DROP INDEX idx_users_name;

        DROP TABLE users;
`,
	)

	cli.SetTime("2025-11-28 15:40")

	// step 1: create the schema via init set
	cli.AssertRun("migrate", "up", "--driver", "test", "--set", "init")
	cli.AssertOutputContains("Applied successfully")
	cli.ResetAllOutputs()

	// step 2: table should not exist in app schema yet
	dataApp := cli.Data("test", "app")
	dataApp.AssertTableNotExists("users")

	// table should not exist in public either
	dataPublic := cli.Data("test", "init")
	dataPublic.AssertTableNotExists("users")

	// step 3: run app set — creates table in app schema
	cli.AssertRun("migrate", "up", "--driver", "test", "--set", "app")
	cli.AssertOutputContains("Applied successfully")
	cli.ResetAllOutputs()

	// table exists in app schema
	dataApp.AssertTableExists("users")

	// table does NOT exist in public schema
	dataPublic.AssertTableNotExists("users")

	// step 4: status checks
	cli.AssertRun("migrate", "status", "--driver", "test", "--set", "app")
	cli.AssertOutputContains("completed v1 create_users")
	cli.ResetAllOutputs()

	// step 5: rollback app set
	cli.AssertRun("migrate", "down", "--driver", "test", "--set", "app", "--version", "v1")
	cli.AssertOutputContains("Migration v1 rolled back successfully")
	cli.ResetAllOutputs()

	dataApp.AssertTableNotExists("users")

	// step 6: rollback init set (drops schema)
	cli.AssertRun("migrate", "down", "--driver", "test", "--set", "init", "--version", "v1")
	cli.AssertOutputContains("Migration v1 rolled back successfully")
	cli.ResetAllOutputs()
}
