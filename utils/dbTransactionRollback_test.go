package utils

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
)

// rollbackStubTx is a pgx.Tx whose Rollback returns err. Any other method panics.
type rollbackStubTx struct {
	pgx.Tx
	err error
}

func (tx rollbackStubTx) Rollback(context.Context) error {
	return tx.err
}

// captureLogs records the standard logger's entries for the rest of the test and restores its
// previous hooks afterwards.
func captureLogs(t *testing.T) *test.Hook {
	t.Helper()

	previousHooks := log.StandardLogger().ReplaceHooks(make(log.LevelHooks))
	t.Cleanup(func() { log.StandardLogger().ReplaceHooks(previousHooks) })
	return test.NewLocal(log.StandardLogger())
}

func TestDeferRollbackIgnoresSuccessAndClosedTx(t *testing.T) {
	tests := map[string]error{
		"rollback succeeds":            nil,
		"tx already committed":         pgx.ErrTxClosed,
		"wrapped tx already committed": fmt.Errorf("traced tx: %w", pgx.ErrTxClosed),
	}
	for name, rollbackErr := range tests {
		t.Run(name, func(t *testing.T) {
			logs := captureLogs(t)

			DeferRollback(rollbackStubTx{err: rollbackErr}, t.Context())

			require.Empty(t, logs.AllEntries())
		})
	}
}

func TestDeferRollbackLogsRollbackFailure(t *testing.T) {
	logs := captureLogs(t)

	DeferRollback(rollbackStubTx{err: errors.New("conn closed")}, t.Context())

	entries := logs.AllEntries()
	require.Len(t, entries, 1)
	require.Equal(t, log.ErrorLevel, entries[0].Level)
	require.Contains(t, entries[0].Message, "conn closed")
}
