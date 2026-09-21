//go:build integration

package mysql

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
	"trading/internal/infrastructure/mysql/dbtest"
	"trading/internal/port"
)

func TestDurableQueueMySQLConcurrencyAndLeaseRecovery(t *testing.T) {
	for _, target := range dbtest.Targets() {
		t.Run(target, func(t *testing.T) {
			db := dbtest.OpenIsolatedMySQL(t, target)
			require.NoError(t, Migrate(db))
			ctx := context.Background()
			queue := NewJobQueue(db)
			store := NewRunStore(db)
			first, err := queue.Enqueue(ctx, queuedRun())
			require.NoError(t, err)
			input := queuedRun()
			input.ID = "duplicate"
			again, err := queue.Enqueue(ctx, input)
			require.NoError(t, err)
			require.Equal(t, first.ID, again.ID)
			var wg sync.WaitGroup
			claims := make(chan port.Run, 20)
			failures := make(chan error, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					run, err := queue.Claim(ctx, fmt.Sprintf("worker%d", i), time.Minute)
					if err == nil {
						claims <- run
					} else {
						failures <- err
					}
				}(i)
			}
			wg.Wait()
			close(claims)
			close(failures)
			require.Len(t, claims, 1)
			old := <-claims
			for err := range failures {
				require.ErrorIs(t, err, port.ErrRunNotFound)
			}
			require.NoError(t, db.Exec("UPDATE t_compute_runs SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE run_id=?", old.ID).Error)
			current, err := queue.Claim(ctx, "second", time.Minute)
			require.NoError(t, err)
			require.Equal(t, 2, current.Attempts)
			require.NotEqual(t, old.LeaseToken, current.LeaseToken)
			require.ErrorIs(t, queue.Renew(ctx, old.ID, old.LeaseToken, time.Minute), port.ErrLeaseLost)
			require.ErrorIs(t, queue.Retry(ctx, old.ID, old.LeaseToken, time.Now().UTC(), port.Failure{Code: "TEMP", Message: "temporary", Retryable: true}), port.ErrLeaseLost)
			require.NoError(t, queue.Renew(ctx, current.ID, current.LeaseToken, time.Minute))
			for attempt := 2; attempt < port.MaxRunAttempts; attempt++ {
				require.NoError(t, queue.Retry(ctx, current.ID, current.LeaseToken, time.Now().UTC().Add(-time.Second), port.Failure{Code: "TEMP", Message: "temporary", Retryable: true}))
				current, err = queue.Claim(ctx, "next", time.Minute)
				require.NoError(t, err)
			}
			require.NoError(t, db.Exec("UPDATE t_compute_runs SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE run_id=?", current.ID).Error)
			_, err = queue.Claim(ctx, "fifth", time.Minute)
			require.ErrorIs(t, err, port.ErrRunNotFound)
			count, err := queue.ReapExpired(ctx)
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			ended, err := store.Get(ctx, current.ID)
			require.NoError(t, err)
			require.Equal(t, port.RunFailed, ended.Status)
			require.Equal(t, port.MaxRunAttempts, ended.Attempts)
			for _, id := range []string{"Key", "key", "key "} {
				run := queuedRun()
				run.ID = id
				run.IdempotencyKey = id
				_, err := queue.Enqueue(ctx, run)
				require.NoError(t, err)
				require.NoError(t, queue.RequestCancel(ctx, id))
				got, err := store.Get(ctx, id)
				require.NoError(t, err)
				require.Equal(t, id, got.ID)
				require.Equal(t, port.RunCancelled, got.Status)
			}
			_, err = queue.Claim(ctx, "none", time.Minute)
			require.ErrorIs(t, err, port.ErrRunNotFound)
		})
	}
}

func seedDurableInstrument(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&InstrumentModel{BaseModel: BaseModel{ID: 41}, Exchange: "SSE", Code: "600000", Source: "test"}).Error)
}
