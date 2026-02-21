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

type BrowserExplainPlan struct {
	NodeType          string               `json:"Node Type"`
	RelationName      string               `json:"Relation Name,omitempty"`
	Alias             string               `json:"Alias,omitempty"`
	Schema            string               `json:"Schema,omitempty"`
	StartupCost       float64              `json:"Startup Cost"`
	TotalCost         float64              `json:"Total Cost"`
	PlanRows          int64                `json:"Plan Rows"`
	PlanWidth         int64                `json:"Plan Width"`
	ActualStartupTime float64              `json:"Actual Startup Time,omitempty"`
	ActualTotalTime   float64              `json:"Actual Total Time,omitempty"`
	ActualRows        int64                `json:"Actual Rows,omitempty"`
	ActualLoops       int64                `json:"Actual Loops,omitempty"`
	SharedHitBlocks   int64                `json:"Shared Hit Blocks,omitempty"`
	SharedReadBlocks  int64                `json:"Shared Read Blocks,omitempty"`
	SharedDirtiedBlocks int64              `json:"Shared Dirtied Blocks,omitempty"`
	SharedWrittenBlocks int64              `json:"Shared Written Blocks,omitempty"`
	Plans             []BrowserExplainPlan `json:"Plans,omitempty"`
}

type BrowserExplainResult struct {
	Plan          BrowserExplainPlan `json:"Plan"`
	PlanningTime  float64            `json:"Planning Time"`
	ExecutionTime float64            `json:"Execution Time"`
}
