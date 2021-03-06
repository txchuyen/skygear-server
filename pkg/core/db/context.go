package db

import (
	"context"
	"net/http"

	"github.com/jmoiron/sqlx"

	"github.com/skygeario/skygear-server/pkg/core/config"
	"github.com/skygeario/skygear-server/pkg/core/errors"
)

type contextKey string

var (
	keyContainer = contextKey("container")
)

// Context provides db with the interface for retrieving an interface to execute sql
type Context interface {
	DB() (ExtContext, error)
}

// TxContext provides the interface for managing transaction
type TxContext interface {
	SafeTxContext

	BeginTx() error
	CommitTx() error
	RollbackTx() error
}

// SafeTxContext only provides interface to check existence of transaction
type SafeTxContext interface {
	HasTx() bool
	EnsureTx()
}

// EndTx implements a common pattern that commit a transaction if no error is
// presented, otherwise rollback the transaction.
func EndTx(tx TxContext, err error) error {
	if err != nil {
		if rbErr := tx.RollbackTx(); rbErr != nil {
			err = errors.WithSecondaryError(err, rbErr)
		}
		return err
	}

	return tx.CommitTx()
}

// WithTx provides a convenient way to wrap a function within a transaction
func WithTx(tx TxContext, do func() error) (err error) {
	if err = tx.BeginTx(); err != nil {
		return
	}

	defer func() {
		err = EndTx(tx, err)
	}()

	err = do()
	return
}

// TODO: handle thread safety
type contextContainer struct {
	pool Pool
	db   *sqlx.DB
	tx   *sqlx.Tx
}

type dbContext struct {
	context.Context
	tConfig config.TenantConfiguration
}

func InitDBContext(ctx context.Context, pool Pool) context.Context {
	container := &contextContainer{pool: pool}
	return context.WithValue(ctx, keyContainer, container)
}

// InitRequestDBContext initialize db context for the request
func InitRequestDBContext(req *http.Request, pool Pool) *http.Request {
	return req.WithContext(InitDBContext(req.Context(), pool))
}

func newDBContext(ctx context.Context, tConfig config.TenantConfiguration) *dbContext {
	return &dbContext{Context: ctx, tConfig: tConfig}
}

// NewContextWithContext creates a new context.DB from context
func NewContextWithContext(ctx context.Context, tConfig config.TenantConfiguration) Context {
	return newDBContext(ctx, tConfig)
}

// NewTxContextWithContext creates a new context.Tx from context
func NewTxContextWithContext(ctx context.Context, tConfig config.TenantConfiguration) TxContext {
	return newDBContext(ctx, tConfig)
}

// NewSafeTxContextWithContext creates a new context.Tx from context
func NewSafeTxContextWithContext(ctx context.Context, tConfig config.TenantConfiguration) SafeTxContext {
	return newDBContext(ctx, tConfig)
}

func (d *dbContext) DB() (ExtContext, error) {
	if d.tx() != nil {
		return d.tx(), nil
	}

	return d.lazydb()
}

func (d *dbContext) HasTx() bool {
	return d.tx() != nil
}

func (d *dbContext) EnsureTx() {
	if d.tx() == nil {
		panic("skydb: a transaction has not begun")
	}
}

func (d *dbContext) BeginTx() error {
	if d.tx() != nil {
		panic("skydb: a transaction has already begun")
	}

	db, err := d.lazydb()
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(d, nil)
	if err != nil {
		return errors.HandledWithMessage(err, "failed to begin transaction")
	}

	container := d.container()
	container.tx = tx

	return nil
}

func (d *dbContext) CommitTx() error {
	if d.tx() == nil {
		panic("skydb: a transaction has not begun")
	}

	err := d.tx().Commit()
	if err != nil {
		return errors.HandledWithMessage(err, "failed to commit transaction")
	}

	container := d.container()
	container.tx = nil
	return nil
}

func (d *dbContext) RollbackTx() error {
	if d.tx() == nil {
		panic("skydb: a transaction has not begun")
	}

	err := d.tx().Rollback()
	if err != nil {
		return errors.HandledWithMessage(err, "failed to rollback transaction")
	}

	container := d.container()
	container.tx = nil
	return nil
}

func (d *dbContext) db() *sqlx.DB {
	return d.container().db
}

func (d *dbContext) tx() *sqlx.Tx {
	return d.container().tx
}

func (d *dbContext) lazydb() (*sqlx.DB, error) {
	db := d.db()
	if db == nil {
		container := d.container()

		var err error
		if db, err = container.pool.Open(d.tConfig); err != nil {
			return nil, errors.HandledWithMessage(err, "failed to connect to database")
		}

		container.db = db
	}

	return db, nil
}

func (d *dbContext) container() *contextContainer {
	return d.Value(keyContainer).(*contextContainer)
}
