package plugin

import "context"

type Browser interface {
	ListNamespaces(ctx context.Context) ([]BrowserNamespace, error)
}

type BrowserNamespace struct {
	ID   string
	Name string
}
