package plugins

import (
	"context"
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

func (b *PostgreSQLBrowser) Show(ctx context.Context, ids []string) error {
	return nil
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
