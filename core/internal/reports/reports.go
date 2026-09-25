package reports

import (
	"context"
	"encoding/json"
	"time"
)

type Record struct {
	ID        string
	Name      string
	FilePath  string
	Format    string
	Status    string
	SHA256    string
	Metadata  json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Store interface {
	CreateReport(ctx context.Context, record Record) error
	GetReport(ctx context.Context, id string) (Record, error)
	ListReports(ctx context.Context, limit int) ([]Record, error)
}
