package plugins

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/qbart/ferrodb/ferro/config"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type PostgreSQLBrowser struct {
	driver *PostgreSQLDriver
	conn   *PostgreSQLDriverConnection
}

func NewPostgreSQLBrowser(driver *PostgreSQLDriver) *PostgreSQLBrowser {
	return &PostgreSQLBrowser{driver: driver}
}

func (b *PostgreSQLBrowser) Connect(ctx context.Context, dsn string) error {
	conn, err := b.driver.Connect(ctx, config.DriverConfig{"dsn": dsn})
	if err != nil {
		return err
	}
	b.conn = conn.(*PostgreSQLDriverConnection)
	return nil
}

func (b *PostgreSQLBrowser) Disconnect(ctx context.Context) error {
	return b.driver.Disconnect(ctx, b.conn)
}

func (b *PostgreSQLBrowser) Show(ctx context.Context, ids []string) (string, error) {
	if len(ids) >= 3 {
		objectType := ids[1]
		switch objectType {
		case "table", "view", "matview", "foreign_table":
			return fmt.Sprintf("SELECT * FROM %s.%s LIMIT 100", ids[0], ids[2]), nil
		case "enum":
			return fmt.Sprintf(
				"SELECT enumlabel AS value FROM pg_enum"+
					" JOIN pg_type ON pg_enum.enumtypid = pg_type.oid"+
					" JOIN pg_namespace ON pg_type.typnamespace = pg_namespace.oid"+
					" WHERE pg_namespace.nspname = '%s' AND pg_type.typname = '%s'"+
					" ORDER BY enumsortorder",
				ids[0], ids[2],
			), nil
		}
	}
	return "", nil
}

func pgOIDToEditorType(oid uint32) string {
	switch oid {
	case 114, 3802: // json, jsonb
		return plugin.EditorTypeObject
	case 2950: // uuid
		return plugin.EditorTypeString
	case 20, 21, 23: // int8, int2, int4
		return plugin.EditorTypeInt64
	case 700, 701, 1700: // float4, float8, numeric
		return plugin.EditorTypeFloat64
	case 26, 28, 29: // oid, xid, cid
		return plugin.EditorTypeUint64
	default:
		return plugin.EditorTypeString
	}
}

func pgValueToString(v any, oid uint32) string {
	if v == nil {
		return "NULL"
	}
	switch oid {
	case 2950: // uuid — pgx returns [16]byte
		if b, ok := v.([16]byte); ok {
			s := hex.EncodeToString(b[:])
			return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
		}
	case 114, 3802: // json, jsonb — pgx may return []byte or already-decoded Go value
		val := v
		if b, ok := v.([]byte); ok {
			var parsed any
			if err := json.Unmarshal(b, &parsed); err != nil {
				return err.Error()
			}
			val = parsed
		}
		marshaled, err := json.Marshal(val)
		if err != nil {
			return err.Error()
		}
		return string(marshaled)
	}
	return fmt.Sprintf("%v", v)
}

func (b *PostgreSQLBrowser) Query(ctx context.Context, sql string) (plugin.BrowserQueryResult, error) {
	rows, err := b.conn.Conn.Query(ctx, sql)
	if err != nil {
		return plugin.BrowserQueryResult{}, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	headers := make([]string, len(fields))
	colTypes := make([]string, len(fields))
	oids := make([]uint32, len(fields))
	for i, f := range fields {
		headers[i] = string(f.Name)
		oids[i] = f.DataTypeOID
		colTypes[i] = pgOIDToEditorType(f.DataTypeOID)
	}

	var data [][]string
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return plugin.BrowserQueryResult{}, err
		}
		row := make([]string, len(vals))
		for i, v := range vals {
			var oid uint32
			if i < len(oids) {
				oid = oids[i]
			}
			row[i] = pgValueToString(v, oid)
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return plugin.BrowserQueryResult{}, err
	}

	return plugin.BrowserQueryResult{Headers: headers, Rows: data, ColumnTypes: colTypes}, nil
}

func (b *PostgreSQLBrowser) List(ctx context.Context, ids []string) ([]plugin.BrowserItem, error) {
	switch len(ids) {
	case 0:
		return b.listSchemas(ctx)
	case 1:
		return []plugin.BrowserItem{
			{ID: "table", Name: "Tables", HasChildren: true},
			{ID: "view", Name: "Views", HasChildren: true},
			{ID: "matview", Name: "Materialized Views", HasChildren: true},
			{ID: "function", Name: "Functions", HasChildren: true},
			{ID: "type", Name: "Types", HasChildren: true},
			{ID: "enum", Name: "Enums", HasChildren: true},
			{ID: "domain", Name: "Domains", HasChildren: true},
			{ID: "composite", Name: "Composite Types", HasChildren: true},
			{ID: "sequence", Name: "Sequences", HasChildren: true},
			{ID: "foreign_table", Name: "Foreign Tables", HasChildren: true},
		}, nil
	case 2:
		schema, objectType := ids[0], ids[1]
		switch objectType {
		case "table":
			return b.listTables(ctx, schema)
		case "view":
			return b.listViews(ctx, schema)
		case "matview":
			return b.listMatViews(ctx, schema)
		case "function":
			return b.listFunctions(ctx, schema)
		case "type":
			return b.listTypes(ctx, schema)
		case "enum":
			return b.listEnums(ctx, schema)
		case "domain":
			return b.listDomains(ctx, schema)
		case "composite":
			return b.listCompositeTypes(ctx, schema)
		case "sequence":
			return b.listSequences(ctx, schema)
		case "foreign_table":
			return b.listForeignTables(ctx, schema)
		}
	case 3:
		schema, objectType, name := ids[0], ids[1], ids[2]
		if objectType == "table" {
			return b.listTableCategories(ctx, schema, name)
		}
	case 4:
		schema, objectType, tableName, category := ids[0], ids[1], ids[2], ids[3]
		if objectType == "table" {
			switch category {
			case "column":
				return b.listColumns(ctx, schema, tableName)
			case "index":
				return b.listIndexes(ctx, schema, tableName)
			case "constraint":
				return b.listConstraints(ctx, schema, tableName)
			case "trigger":
				return b.listTriggers(ctx, schema, tableName)
			case "partition":
				return b.listPartitions(ctx, schema, tableName)
			case "policy":
				return b.listPolicies(ctx, schema, tableName)
			}
		}
	}
	return nil, fmt.Errorf("unsupported path depth: %d", len(ids))
}

func (b *PostgreSQLBrowser) listSchemas(ctx context.Context) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx, `SELECT schema_name FROM information_schema.schemata ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: true}, nil
	})
}

func (b *PostgreSQLBrowser) listTables(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT table_name FROM information_schema.tables WHERE table_schema = $1 AND table_type = 'BASE TABLE' ORDER BY table_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: true}, nil
	})
}

func (b *PostgreSQLBrowser) listTableCategories(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	categories := []plugin.BrowserItem{
		{ID: "column", Name: "Columns", HasChildren: true},
		{ID: "index", Name: "Indexes", HasChildren: true},
		{ID: "constraint", Name: "Constraints", HasChildren: true},
		{ID: "trigger", Name: "Triggers", HasChildren: true},
		{ID: "policy", Name: "Policies", HasChildren: true},
	}

	var relkind string
	err := b.conn.Conn.QueryRow(ctx,
		`SELECT c.relkind FROM pg_class c JOIN pg_namespace n ON c.relnamespace = n.oid WHERE n.nspname = $1 AND c.relname = $2`,
		schema, table,
	).Scan(&relkind)
	if err == nil && relkind == "p" {
		categories = append(categories, plugin.BrowserItem{ID: "partition", Name: "Partitions", HasChildren: true})
	}

	return categories, nil
}

func (b *PostgreSQLBrowser) listColumns(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listIndexes(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT indexname FROM pg_indexes WHERE schemaname = $1 AND tablename = $2 ORDER BY indexname`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listConstraints(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT constraint_name FROM information_schema.table_constraints WHERE table_schema = $1 AND table_name = $2 ORDER BY constraint_name`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listTriggers(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT DISTINCT trigger_name FROM information_schema.triggers WHERE trigger_schema = $1 AND event_object_table = $2 ORDER BY trigger_name`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listPartitions(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT c.relname FROM pg_inherits i JOIN pg_class c ON i.inhrelid = c.oid JOIN pg_class p ON i.inhparent = p.oid JOIN pg_namespace n ON p.relnamespace = n.oid WHERE n.nspname = $1 AND p.relname = $2 ORDER BY c.relname`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listPolicies(ctx context.Context, schema, table string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT policyname FROM pg_policies WHERE schemaname = $1 AND tablename = $2 ORDER BY policyname`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listViews(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT table_name FROM information_schema.views WHERE table_schema = $1 ORDER BY table_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listMatViews(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT matviewname FROM pg_matviews WHERE schemaname = $1 ORDER BY matviewname`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listFunctions(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT routine_name FROM information_schema.routines WHERE routine_schema = $1 AND routine_type = 'FUNCTION' ORDER BY routine_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listTypes(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT t.typname FROM pg_type t JOIN pg_namespace n ON t.typnamespace = n.oid WHERE n.nspname = $1 AND t.typtype = 'r' ORDER BY t.typname`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listEnums(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT t.typname FROM pg_type t JOIN pg_namespace n ON t.typnamespace = n.oid WHERE n.nspname = $1 AND t.typtype = 'e' ORDER BY t.typname`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listDomains(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT t.typname FROM pg_type t JOIN pg_namespace n ON t.typnamespace = n.oid WHERE n.nspname = $1 AND t.typtype = 'd' ORDER BY t.typname`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listCompositeTypes(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT t.typname FROM pg_type t JOIN pg_namespace n ON t.typnamespace = n.oid JOIN pg_class c ON t.typrelid = c.oid WHERE n.nspname = $1 AND t.typtype = 'c' AND c.relkind = 'c' ORDER BY t.typname`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listSequences(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT sequence_name FROM information_schema.sequences WHERE sequence_schema = $1 ORDER BY sequence_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}

func (b *PostgreSQLBrowser) listForeignTables(ctx context.Context, schema string) ([]plugin.BrowserItem, error) {
	rows, err := b.conn.Conn.Query(ctx,
		`SELECT foreign_table_name FROM information_schema.foreign_tables WHERE foreign_table_schema = $1 ORDER BY foreign_table_name`,
		schema,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserItem, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserItem{}, err
		}
		return plugin.BrowserItem{ID: name, Name: name, HasChildren: false}, nil
	})
}
