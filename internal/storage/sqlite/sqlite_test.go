package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/common"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/curriculum"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/identity"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/job"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/offering"
	"github.com/11DingKing/ai-learning-pathways-go/internal/domain/resource"
	"github.com/11DingKing/ai-learning-pathways-go/internal/repository"
	"github.com/11DingKing/ai-learning-pathways-go/internal/storage/sqlite"
)

var now = time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)

func openDB(t *testing.T) (*sqlite.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pathways.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return db, path
}

func seedIdentity(t *testing.T, db *sqlite.DB) (common.ID, identity.User) {
	t.Helper()
	tenantID := common.ID("tenant_test")
	userID := common.ID("user_admin")
	user, err := identity.NewUser(userID, tenantID, "admin@example.edu", "Admin", "bcrypt-hash", identity.RoleProgramAdmin, identity.StageUniversity, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BootstrapIdentity(context.Background(), tenantID, "Test Tenant", "UTC", user); err != nil {
		t.Fatal(err)
	}
	return tenantID, user
}

func TestMigrationReopenAndSessionPersistence(t *testing.T) {
	db, path := openDB(t)
	tenantID, user := seedIdentity(t, db)
	token := "abcdefghijklmnopqrstuvwxyz123456"
	session, err := identity.NewSession(common.ID("session_test"), tenantID, user.ID, token, now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.InsertSession(ctx, session) }); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Read(context.Background(), func(ctx context.Context, reader repository.Reader) error {
		loaded, err := reader.GetSessionByTokenHash(ctx, session.TokenHash)
		if err != nil {
			return err
		}
		if loaded.ID != session.ID || !loaded.ExpiresAt.Equal(session.ExpiresAt) {
			t.Fatalf("session changed after reopen: %+v", loaded)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTransactionRollbackKeepsAllEntitiesUnchanged(t *testing.T) {
	db, _ := openDB(t)
	tenantID, user := seedIdentity(t, db)
	defer db.Close()
	value, err := curriculum.New(common.ID("cur_rollback"), tenantID, user.ID, "AI-ROLLBACK", "Rollback curriculum", "This curriculum has enough detail to make transaction rollback observable and safe.", identity.StageUniversity, now)
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error {
		if err := tx.InsertCurriculum(ctx, value); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if err := db.Read(context.Background(), func(ctx context.Context, reader repository.Reader) error {
		_, err := reader.GetCurriculum(ctx, tenantID, value.ID)
		if !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("rolled back curriculum found: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOptimisticVersionConflict(t *testing.T) {
	db, _ := openDB(t)
	tenantID, user := seedIdentity(t, db)
	defer db.Close()
	value, err := curriculum.New(common.ID("cur_conflict"), tenantID, user.ID, "AI-CONFLICT", "Conflict curriculum", "This curriculum has enough detail to exercise optimistic version conflict behavior.", identity.StageUniversity, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.InsertCurriculum(ctx, value) }); err != nil {
		t.Fatal(err)
	}
	first, second := value, value
	first.Title = "First update"
	first.Version = 2
	first.UpdatedAt = now.Add(time.Minute)
	second.Title = "Second update"
	second.Version = 2
	second.UpdatedAt = now.Add(2 * time.Minute)
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.UpdateCurriculum(ctx, first, 1) }); err != nil {
		t.Fatal(err)
	}
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error { return tx.UpdateCurriculum(ctx, second, 1) }); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestOfferingSeatCapacityUnderConcurrentTransactions(t *testing.T) {
	db, _ := openDB(t)
	tenantID, admin := seedIdentity(t, db)
	defer db.Close()
	course, err := curriculum.New(common.ID("cur_seats"), tenantID, admin.ID, "AI-SEATS", "Seat curriculum", "This curriculum has enough detail to exercise conditional capacity updates.", identity.StageUniversity, now)
	if err != nil {
		t.Fatal(err)
	}
	course.Status = curriculum.StatusPublished
	window, _ := common.NewWindow(now, now.Add(time.Hour))
	offeringValue, err := offering.New(common.ID("offering_seats"), tenantID, course.ID, admin.ID, "SEATS", "Seat Offering", 1, window, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	offeringValue.Status = offering.StatusActive
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error {
		if err := tx.InsertCurriculum(ctx, course); err != nil {
			return err
		}
		return tx.InsertOffering(ctx, offeringValue)
	}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error {
				_, err := tx.ReserveOfferingSeat(ctx, tenantID, offeringValue.ID, 1)
				return err
			})
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var successes int
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, common.ErrConflict) && !errors.Is(err, common.ErrCapacity) {
			t.Fatalf("unexpected seat result: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one seat reservation, got %d", successes)
	}
	if err := db.Read(context.Background(), func(ctx context.Context, reader repository.Reader) error {
		value, err := reader.GetOffering(ctx, tenantID, offeringValue.ID)
		if err != nil {
			return err
		}
		if value.ConfirmedSeats != 1 {
			t.Fatalf("confirmed seats exceeded capacity: %d", value.ConfirmedSeats)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestResourceDeliveryAndJobClaimsAreExclusive(t *testing.T) {
	db, _ := openDB(t)
	tenantID, admin := seedIdentity(t, db)
	defer db.Close()
	pack, err := resource.NewPack(common.ID("pack_test"), tenantID, "Teaching Pack", "CC-BY", "https://example.edu/pack", "0123456789012345678901234567890123456789012345678901234567890123", "primary", "university", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := pack.Approve(admin.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := pack.Publish(now); err != nil {
		t.Fatal(err)
	}
	window, _ := common.NewWindow(now, now.Add(time.Hour))
	course, _ := curriculum.New(common.ID("cur_resource"), tenantID, admin.ID, "AI-RESOURCE", "Resource curriculum", "This curriculum has enough detail to exercise resource delivery and jobs.", identity.StageUniversity, now)
	course.Status = curriculum.StatusPublished
	offeringValue, _ := offering.New(common.ID("offering_resource"), tenantID, course.ID, admin.ID, "RESOURCE", "Resource Offering", course.Version, window, 2, now)
	offeringValue.Status = offering.StatusActive
	learnerID := common.ID("learner_resource")
	learner, _ := identity.NewUser(learnerID, tenantID, "learner@example.edu", "Learner", "hash", identity.RoleLearner, identity.StageUniversity, now)
	enrollment, _ := offering.NewEnrollment(common.ID("enrollment_resource"), tenantID, offeringValue.ID, learnerID, "resource-request", now)
	_ = enrollment.Confirm(now)
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error {
		if err := tx.InsertUser(ctx, learner); err != nil {
			return err
		}
		if err := tx.InsertCurriculum(ctx, course); err != nil {
			return err
		}
		if err := tx.InsertOffering(ctx, offeringValue); err != nil {
			return err
		}
		if err := tx.InsertEnrollment(ctx, enrollment); err != nil {
			return err
		}
		if err := tx.InsertResourcePack(ctx, pack); err != nil {
			return err
		}
		delivery, err := resource.NewDelivery(common.ID("delivery_test"), tenantID, pack.ID, offeringValue.ID, learnerID, "pack:offering:recipient", now)
		if err != nil {
			return err
		}
		if err := tx.InsertDelivery(ctx, delivery); err != nil {
			return err
		}
		jobValue, err := job.New(common.ID("job_test"), tenantID, "resource.deliver", "delivery", delivery.ID, map[string]string{"delivery_id": delivery.ID.String()}, now, 3, now)
		if err != nil {
			return err
		}
		return tx.InsertJob(ctx, jobValue)
	}); err != nil {
		t.Fatal(err)
	}
	var claimed []job.Job
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error {
		var err error
		claimed, err = tx.ClaimJobs(ctx, "worker-a", now, time.Minute, []string{"resource.deliver"}, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed job, got %d", len(claimed))
	}
	var claimedAgain []job.Job
	if err := db.WithinTx(context.Background(), func(ctx context.Context, tx repository.Tx) error {
		var err error
		claimedAgain, err = tx.ClaimJobs(ctx, "worker-b", now, time.Minute, []string{"resource.deliver"}, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("leased job was claimed twice: %d", len(claimedAgain))
	}
	if err := db.Read(context.Background(), func(ctx context.Context, reader repository.Reader) error {
		value, err := reader.GetDelivery(ctx, tenantID, common.ID("delivery_test"))
		if err != nil {
			return err
		}
		if value.Status != resource.DeliveryPending {
			t.Fatalf("delivery should remain pending until handler transition: %s", value.Status)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestContextCancellationStopsTransaction(t *testing.T) {
	db, _ := openDB(t)
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := db.WithinTx(ctx, func(context.Context, repository.Tx) error { t.Fatal("callback must not run"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
