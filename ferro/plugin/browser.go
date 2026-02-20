package plugin

import "context"

type Browser interface {
	Connect(ctx context.Context, dsn string) error
	Disconnect(ctx context.Context) error
	List(ctx context.Context, ids []string) ([]BrowserItem, error)
	Show(ctx context.Context, ids []string) error
}

type BrowserItem struct {
	ID          string
	Name        string
	HasChildren bool
}
