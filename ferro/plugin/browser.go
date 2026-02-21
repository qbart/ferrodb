package plugin

import "context"

const (
	EditorTypeString  = "string"
	EditorTypeFloat64 = "float64"
	EditorTypeInt64   = "int64"
	EditorTypeUint64  = "uint64"
	EditorTypeObject  = "object"
)

type Browser interface {
	Connect(ctx context.Context, dsn string) error
	Disconnect(ctx context.Context) error
	List(ctx context.Context, ids []string) ([]BrowserItem, error)
	Show(ctx context.Context, ids []string) (string, error)
	Query(ctx context.Context, sql string) (BrowserQueryResult, error)
	ParseExplain(data BrowserQueryResult) (BrowserExplainResult, error)
}

type BrowserItem struct {
	ID          string
	Name        string
	HasChildren bool
}

type BrowserQueryResult struct {
	Headers     []string
	Rows        [][]string
	ColumnTypes []string
}

type BrowserExplainLine struct {
	Text      string
	Highlight bool
}

type BrowserExplainNode struct {
	Name     string
	Lines    []BrowserExplainLine
	Children []BrowserExplainNode
}

type BrowserExplainResult struct {
	Root              BrowserExplainNode
	PlanningTime      float64
	ExecutionTime     float64
	TotalSortMemoryKB int64
	TotalHashMemoryKB int64
}
