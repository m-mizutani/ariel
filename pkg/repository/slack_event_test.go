package repository_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

func newClaim(eventID string) *model.SlackEventClaim {
	now := time.Now().UTC()
	return &model.SlackEventClaim{EventID: eventID, ClaimedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
}

func TestSlackEventRepository(t *testing.T) {
	runRepositoryTest(t, "first claim wins", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		eventID := "Ev" + randomSuffix(t)

		first, err := repo.SlackEvent().Claim(ctx, newClaim(eventID))
		gt.NoError(t, err).Required()
		gt.Bool(t, first).True()

		second, err := repo.SlackEvent().Claim(ctx, newClaim(eventID))
		gt.NoError(t, err).Required()
		gt.Bool(t, second).False()
	})

	runRepositoryTest(t, "different IDs are claimed independently", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		a, err := repo.SlackEvent().Claim(ctx, newClaim("Ev"+randomSuffix(t)))
		gt.NoError(t, err).Required()
		b, err := repo.SlackEvent().Claim(ctx, newClaim("Ev"+randomSuffix(t)))
		gt.NoError(t, err).Required()
		gt.Bool(t, a).True()
		gt.Bool(t, b).True()
	})

	runRepositoryTest(t, "concurrent claims succeed exactly once", func(t *testing.T, repo interfaces.Repository) {
		ctx := testContext(t)
		eventID := "Ev" + randomSuffix(t)

		var wins atomic.Int32
		var failures atomic.Int32
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := repo.SlackEvent().Claim(ctx, newClaim(eventID))
				if err != nil {
					failures.Add(1)
					return
				}
				if ok {
					wins.Add(1)
				}
			}()
		}
		wg.Wait()
		gt.Number(t, failures.Load()).Equal(0)
		gt.Number(t, wins.Load()).Equal(1)
	})

	runRepositoryTest(t, "invalid claim is rejected", func(t *testing.T, repo interfaces.Repository) {
		_, err := repo.SlackEvent().Claim(testContext(t), newClaim("Ev/1"))
		gt.Value(t, err).NotNil()
	})
}
