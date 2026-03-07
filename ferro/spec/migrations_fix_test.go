package spec

import (
	"fmt"
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
		Metadata: map[string]any{
			"checksum": cli.Checksum("create_animals"),
		},
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
			"error":    fmt.Sprintf(`exec: failed to execute query: %s`, migrationError()),
			"checksum": cli.Checksum("create_animals"),
		},
	})

	// at this point we have broken state,
	// only up fix should be allowed,
	// other ops should not be permitted
	//
	// retry should not even start
	cli.AssertNotRun("migrate", "up", "--driver", "test", "--set", "public")
	cli.AssertErrorContains("Migration create_animals is in a failed state, please fix the migration before proceeding")
	cli.ResetAllOutputs()

	// migration down should not work as well
	cli.AssertNotRun("migrate", "down", "--driver", "test", "--set", "public", "--version", "v1")
	cli.AssertErrorContains("Migration create_animals is in a failed state, please fix the migration before proceeding")
	cli.ResetAllOutputs()

	// down fix should not work
	cli.AssertNotRun("migrate", "fix", "down", "--driver", "test", "--set", "public", "--version", "v1")
	cli.AssertErrorContains("Migration with version v1 failed when applying, not during rollback, fix it accordingly")
	cli.ResetAllOutputs()

	// and finally up fix should work
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
		Metadata: map[string]any{
			"checksum": cli.Checksum("create_animals"),
		},
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
			"error":    fmt.Sprintf(`exec: failed to execute query: %s`, migrationError()),
			"checksum": cli.Checksum("create_animals"),
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

	// now we fix the file and reapply migration like the human would do
	data.AssertTableNotExists("animals")
	cli.Files(
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
      sql: CREATE TABLE animals(id integer);
    down:
      sql: DROP TABLE animals;
`,
	)
	cli.AssertRun("migrate", "up", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("Applied successfully")
	cli.ResetAllOutputs()
	data.AssertTableExists("animals")

	audit = cli.Audit("test", "public")
	audit.AssertCount(5)
	cli.AssertRun("migrate", "audit", "--driver", "test", "--set", "public", "-f", "long")
	cli.AssertOutputContains(fmt.Sprintf(`
          1 2025-11-28 15:40:00 migration.up.started create_animals v1
        ! 2 2025-11-28 15:40:00 migration.up.failed create_animals v1
            exec: failed to execute query: %s
        ~ 3 2025-11-28 15:40:00 migration.up.fixed create_animals v1
            fixed by adding table name and re-run
          4 2025-11-28 15:40:00 migration.up.started create_animals v1
        + 5 2025-11-28 15:40:00 migration.up.completed create_animals v1
        `, migrationError()))
}

func TestMigrationsFixDown(t *testing.T) {
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
      sql: CREATE TABLE animals(id integer);
    down:
      sql: DROP TABLE;
`,
	)

	cli.SetTime("2025-11-28 15:40")

	cli.AssertRun("migrate", "up", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("Applied successfully")
	cli.ResetAllOutputs()

	data := cli.Data("test", "public")
	data.AssertTableExists("animals")

	cli.AssertNotRun("migrate", "down", "--driver", "test", "--set", "public", "--version", "v1")
	cli.AssertErrorContains(`exec: failed to execute query`)
	cli.ResetAllOutputs()

	audit := cli.Audit("test", "public")
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
		Event: "migration.down.failed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"error":    fmt.Sprintf(`exec: failed to execute query: %s`, migrationDownError()),
			"checksum": cli.Checksum("create_animals"),
		},
	})

	// at this point we have broken state,
	// only down fix is allowed,
	// other ops should not be permitted
	//
	// retry should not even start
	cli.AssertNotRun("migrate", "down", "--driver", "test", "--set", "public", "--version", "v1")
	cli.AssertErrorContains("Migration create_animals is in a failed state, please fix the migration before proceeding")
	cli.ResetAllOutputs()

	// migration up should not work as well
	cli.AssertNotRun("migrate", "up", "--driver", "test", "--set", "public")
	cli.AssertErrorContains("Migration create_animals is in a failed state, please fix the migration before proceeding")
	cli.ResetAllOutputs()

	// up fix should not work
	cli.AssertNotRun("migrate", "fix", "up", "--driver", "test", "--set", "public", "--version", "v1")
	cli.AssertErrorContains("Migration with version v1 failed during rollback, not when applying, fix it accordingly")
	cli.ResetAllOutputs()

	// and finally down fix should work
	cli.AssertRun("migrate", "fix", "down", "--driver", "test", "--set", "public", "--version", "v1", "-C", "manually fixed")
	cli.AssertOutputContains("Marked as fixed successfully")
	cli.ResetAllOutputs()

	audit = cli.Audit("test", "public")
	audit.AssertCount(5)
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
		Event: "migration.down.failed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"error":    fmt.Sprintf(`exec: failed to execute query: %s`, migrationDownError()),
			"checksum": cli.Checksum("create_animals"),
		},
	})
	audit.Assert(4, auditLog{
		ID:    5,
		Event: "migration.down.fixed",
		Data: map[string]any{
			"migration": "create_animals",
			"set":       "public",
			"version":   "v1",
		},
		Metadata: map[string]any{
			"comment": "manually fixed",
		},
	})

	cli.AssertRun("migrate", "status", "--driver", "test", "--set", "public")
	cli.AssertOutputContains("completed v1 create_animals")
	cli.ResetAllOutputs()

	// now we fix the file and rollback migration like the human would do
	data.AssertTableExists("animals")
	cli.Files(
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
      sql: CREATE TABLE animals();
    down:
      sql: DROP TABLE animals;
`,
	)
	cli.AssertRun("migrate", "down", "--driver", "test", "--set", "public", "--version", "v1")
	cli.AssertOutputContains("Migration v1 rolled back successfully")
	cli.ResetAllOutputs()
	data.AssertTableNotExists("animals")

	audit = cli.Audit("test", "public")
	audit.AssertCount(7)
	cli.AssertRun("migrate", "audit", "--driver", "test", "--set", "public", "-f", "long")
	cli.AssertOutputContains(fmt.Sprintf(`
          1 2025-11-28 15:40:00 migration.up.started create_animals v1
        + 2 2025-11-28 15:40:00 migration.up.completed create_animals v1
          3 2025-11-28 15:40:00 migration.down.started create_animals v1
        ! 4 2025-11-28 15:40:00 migration.down.failed create_animals v1
            exec: failed to execute query: %s
        ~ 5 2025-11-28 15:40:00 migration.down.fixed create_animals v1
            manually fixed
          6 2025-11-28 15:40:00 migration.down.started   create_animals v1
        - 7 2025-11-28 15:40:00 migration.down.completed create_animals v1
        `, migrationDownError()))
}
