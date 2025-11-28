package spec

import (
	"testing"
)

func TestMigrationsFixUp(t *testing.T) {
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
		"01.fyml",
		`
apiVersion: migrations/v1
kind: Migration
metadata:
  name: create_animals
spec:
  version: "v1"
  run:
    up:
      sql: CREATE TABLE;
    down:
      sql: DROP TABLE animals;
`,
	)

	cli.SetTime("2025-11-28 15:40")

	cli.AssertNotRun("migrate", "up", "--driver", "test", "--set", "public")
	cli.AssertErrorContains("driver failed to run: exec: failed to execute query")
	cli.ResetAllOutputs()

	data := cli.Data("test", "public")
	data.AssertTableNotExists("animals")

	cli.AssertRun("migrate", "status", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("failed v1 create_animals")
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
		Metadata: map[string]any{},
	})
	audit.Assert(1, auditLog{
		ID:    2,
		Event: "migration.up.failed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
            "error": `exec: failed to execute query: ERROR: syntax error at or near ";" (SQLSTATE 42601)`,
        },
	})

    // at this point we have broken state,
    // other ops should not permitted
	cli.AssertNotRun("migrate", "up", "--driver", "test", "--set", "public")
	cli.AssertErrorContains("Migration create_animals is in a failed state, please fix the migration before proceeding")
	cli.ResetAllOutputs()

	cli.AssertRun("migrate", "fix", "up", "--driver", "test", "--set", "public", "--version", "v1", "-C", "fixed by adding table name and re-run")
	cli.AssertOutputContains("Marked as fixed successfully")
	cli.ResetAllOutputs()

	audit = cli.Audit("test", "public")
	audit.AssertCount(3)
	audit.Assert(0, auditLog{
		ID:    1,
		Event: "migration.up.started",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{},
	})
	audit.Assert(1, auditLog{
		ID:    2,
		Event: "migration.up.failed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
            "error": `exec: failed to execute query: ERROR: syntax error at or near ";" (SQLSTATE 42601)`,
        },
	})
	audit.Assert(2, auditLog{
		ID:    3,
		Event: "migration.up.fixed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
            "comment": "fixed by adding table name and re-run",
        },
	})

	cli.AssertRun("migrate", "status", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("pending v1 create_animals")
	cli.ResetAllOutputs()

	cli.AssertRun("migrate", "audit", "--driver", "test", "--set", "public")
	cli.AssertOutputContains(`
          1 2025-11-28 15:40:00 migration.up.started create_animals v1
        ! 2 2025-11-28 15:40:00 migration.up.failed create_animals v1
        ~ 3 2025-11-28 15:40:00 migration.up.fixed create_animals v1
        `)
}
