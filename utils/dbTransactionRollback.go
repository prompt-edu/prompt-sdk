package utils

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
)

// DeferRollback rolls back tx and logs a failed rollback. Defer it right after Begin: once tx has
// been committed, the rollback returns pgx.ErrTxClosed, which is expected and not logged.
func DeferRollback(tx pgx.Tx, ctx context.Context) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		log.Error("Error rolling back transaction: ", err)
	}
}
