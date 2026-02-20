package plugins

import (
	"context"

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

func (b *PostgreSQLBrowser) ListNamespaces(ctx context.Context) ([]plugin.BrowserNamespace, error) {
	rows, err := b.conn.Conn.Query(ctx, `SELECT schema_name FROM information_schema.schemata ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (plugin.BrowserNamespace, error) {
		var name string
		if err := row.Scan(&name); err != nil {
			return plugin.BrowserNamespace{}, err
		}
		return plugin.BrowserNamespace{ID: name, Name: name}, nil
	})
}

func (b *PostgreSQLBrowser) ListNamespaceObjects(ctx context.Context) ([]plugin.BrowserNamespaceObject, error) {
	return []plugin.BrowserNamespaceObject{
		{ID: "table", Name: "Tables"},
		{ID: "view", Name: "Views"},
	}, nil
}
