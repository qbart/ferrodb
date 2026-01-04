package spec

import (
	"testing"
)

func TestMigrationsHappyPath(t *testing.T) {
	cli, teardown := NewTestCLI(t)
	defer teardown()

	dbTeardown := cli.RandomDatabase()
	defer dbTeardown()

	cli.Files(
		"set.fyml",
		`
apiVersion: migrations/v1
kind: MigrationSet
metadata:
  name: public
spec:
  namespace:
    name: public
  migrations:
    - create_animals
        `,
		"01_create_animals.fyml",
		`
apiVersion: migrations/v1
kind: Migration
metadata:
  name: create_animals
spec:
  version: "v1"
  run:
    up:
      sql: CREATE TABLE animals();
    down:
      sql: DROP TABLE animals;
`,
	)

	cli.SetTime("2025-11-28 15:40")

	cli.AssertRun("migrate", "status", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("pending v1 create_animals")
	cli.ResetAllOutputs()

	data := cli.Data("test", "public")
	data.AssertTableNotExists("animals")

	cli.AssertRun("migrate", "up", "--driver", "test", "--set", "public")
	cli.AssertOutputNotContains("No pending migrations")
	cli.AssertOutputContains("Applied successfully")
	cli.ResetAllOutputs()

	cli.AssertRun("migrate", "up", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("No pending migrations")
	cli.ResetAllOutputs()

	cli.AssertRun("migrate", "status", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("completed v1 create_animals")
	cli.ResetAllOutputs()

	audit := cli.Audit("test", "public")
	audit.AssertCount(2)
	audit.Assert(0, auditLog{
		ID:    1,
		Event: "migration.up.started",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"checksum": cli.Checksum("create_animals"),
		},
	})
	audit.Assert(1, auditLog{
		ID:    2,
		Event: "migration.up.completed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"checksum": cli.Checksum("create_animals"),
		},
	})
	data.AssertTableExists("animals")

	cli.AssertRun("migrate", "down", "--driver", "test", "--set", "public", "--version", "v1")
	cli.AssertOutputContains("Migration v1 rolled back successfully")
	cli.ResetAllOutputs()

	audit = cli.Audit("test", "public")
	audit.AssertCount(4)
	audit.Assert(0, auditLog{
		ID:    1,
		Event: "migration.up.started",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"checksum": cli.Checksum("create_animals"),
		},
	})
	audit.Assert(1, auditLog{
		ID:    2,
		Event: "migration.up.completed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"checksum": cli.Checksum("create_animals"),
		},
	})
	audit.Assert(2, auditLog{
		ID:    3,
		Event: "migration.down.started",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"checksum": cli.Checksum("create_animals"),
		},
	})
	audit.Assert(3, auditLog{
		ID:    4,
		Event: "migration.down.completed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"checksum": cli.Checksum("create_animals"),
		},
	})
	data.AssertTableNotExists("animals")

	cli.AssertRun("migrate", "status", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("pending v1 create_animals")
	cli.ResetAllOutputs()

	cli.AssertRun("migrate", "audit", "--driver", "test", "--set", "public")
	cli.AssertOutputContains(`
          1 2025-11-28 15:40:00 migration.up.started create_animals v1
        + 2 2025-11-28 15:40:00 migration.up.completed create_animals v1
          3 2025-11-28 15:40:00 migration.down.started create_animals v1
        - 4 2025-11-28 15:40:00 migration.down.completed create_animals v1
        `)
}

