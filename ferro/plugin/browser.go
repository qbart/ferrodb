package plugin

import "context"

type Browser interface {
	ListNamespaces(ctx context.Context) ([]BrowserNamespace, error)
	ListNamespaceObjects(ctx context.Context) ([]BrowserNamespaceObject, error)
}

type BrowserNamespace struct {
	ID   string
	Name string
}

type BrowserNamespaceObject struct {
	ID   string
	Name string
}
