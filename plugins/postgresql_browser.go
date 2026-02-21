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
		return fmt.Sprintf("SELECT * FROM %s.%s LIMIT 100", ids[0], ids[2]), nil
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
	case 114, 3802: // json, jsonb — pgx returns []byte
		if b, ok := v.([]byte); ok {
			var parsed any
			if err := json.Unmarshal(b, &parsed); err != nil {
				return err.Error()
			}
			marshaled, err := json.Marshal(parsed)
			if err != nil {
				return err.Error()
			}
			return string(marshaled)
		}
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
		}, nil
	case 2:
		schema, objectType := ids[0], ids[1]
		switch objectType {
		case "table":
			return b.listTables(ctx, schema)
		case "view":
			return b.listViews(ctx, schema)
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
