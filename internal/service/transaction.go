package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const rollbackTimeout = 5 * time.Second

type transactionBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

func inTransaction[T any](
	ctx context.Context,
	db transactionBeginner,
	operation func(pgx.Tx) (T, error),
) (result T, returnErr error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), rollbackTimeout)
		defer cancel()

		rollbackErr := tx.Rollback(rollbackCtx)
		if rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return
		}

		rollbackErr = fmt.Errorf("rollback transaction: %w", rollbackErr)
		if returnErr == nil {
			var zero T
			result = zero
			returnErr = rollbackErr
			return
		}
		returnErr = errors.Join(returnErr, rollbackErr)
	}()

	result, err = operation(tx)
	if err != nil {
		var zero T
		return zero, err
	}

	if err := tx.Commit(ctx); err != nil {
		var zero T
		return zero, fmt.Errorf("commit transaction: %w", err)
	}

	return result, nil
}
