package db

import (
	"context"

	"github.com/skygeario/skygear-server/pkg/core/config"
)

type DBProvider struct{}

func (p DBProvider) Provide(ctx context.Context, tConfig config.TenantConfiguration) IDB {
	return &DB{tConfig.DBConnectionStr}
}

type IDB interface {
	GetRecord(string) string
}

type DB struct {
	ConnectionStr string
}

func (db DB) GetRecord(recordID string) string {
	return db.ConnectionStr + ":" + recordID
}