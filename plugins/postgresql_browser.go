package plugins

import (
	"context"

	"github.com/qbart/ferrodb/ferro/plugin"
)

type PostgreSQLBrowser struct {
	driver *PostgreSQLDriver
}

func NewPostgreSQLBrowser(driver *PostgreSQLDriver) *PostgreSQLBrowser {
	return &PostgreSQLBrowser{}
}

func (b *PostgreSQLBrowser) ListNamespaces(ctx context.Context) ([]plugin.BrowserNamespace, error) {
	return []plugin.BrowserNamespace{
		{"public", "public"},
		{"tenant2", "tenant2"},
		{"tenant2", "tenant2"},
	}, nil
}
