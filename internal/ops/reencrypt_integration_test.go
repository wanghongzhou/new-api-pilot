package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

var (
	reencryptTestOldKey = []byte("abcdefghijklmnopqrstuvwxyz123456")
	reencryptTestNewKey = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	reencryptTestAltKey = []byte("01234567890123456789012345678901")
)

func TestMySQLReencryptDryRunAndSuccess(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)
	oldCipher := mustTestCipher(t, reencryptTestOldKey)
	newCipher := mustTestCipher(t, reencryptTestNewKey)
	fixture := newReencryptFixture(t, ctx, database, oldCipher, 1, true)

	dryRun, err := RunReencrypt(ctx, ReencryptOptions{
		Database: database, OldCipher: oldCipher, NewCipher: newCipher,
		BatchSize: 1, DryRun: true, Now: time.Now,
	})
	if err != nil || dryRun.Status != "success" || dryRun.Counts.OldKey != 2 || dryRun.Counts.Updated != 0 {
		t.Fatalf("dry-run report = %#v, error = %v", dryRun, err)
	}
	fixture.assertBusinessCiphertexts(t, ctx, oldCipher, newCipher, true)
	fixture.assertNoMaintenanceState(t, ctx)

	report, err := RunReencrypt(ctx, ReencryptOptions{
		Database: database, OldCipher: oldCipher, NewCipher: newCipher,
		BatchSize: 1, Now: time.Now,
	})
	if err != nil || report.Status != "success" || report.Counts.Updated != 2 || report.Resumed {
		t.Fatalf("re-encryption report = %#v, error = %v", report, err)
	}
	fixture.assertBusinessCiphertexts(t, ctx, oldCipher, newCipher, false)
	fixture.assertNoStagedItems(t, ctx)
}

func TestMySQLReencryptResumesAndRejectsDifferentKeyPair(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)
	oldCipher := mustTestCipher(t, reencryptTestOldKey)
	newCipher := mustTestCipher(t, reencryptTestNewKey)
	altCipher := mustTestCipher(t, reencryptTestAltKey)
	fixture := newReencryptFixture(t, ctx, database, oldCipher, 2, false)

	interrupted, err := RunReencrypt(ctx, ReencryptOptions{
		Database: database, OldCipher: oldCipher, NewCipher: newCipher,
		BatchSize: 1, Now: time.Now,
		AfterStageBatch: func(int64) error { return errors.New("injected interruption") },
	})
	if err == nil || interrupted.Error == nil || interrupted.Error.Code != "REENCRYPT_INTERRUPTED" {
		t.Fatalf("interrupted report = %#v, error = %v", interrupted, err)
	}
	fixture.assertBusinessCiphertexts(t, ctx, oldCipher, newCipher, true)

	mismatch, err := RunReencrypt(ctx, ReencryptOptions{
		Database: database, OldCipher: oldCipher, NewCipher: altCipher,
		BatchSize: 1, Now: time.Now,
	})
	if err == nil || mismatch.Error == nil || mismatch.Error.Code != "REENCRYPT_ACTIVE_JOB_MISMATCH" {
		t.Fatalf("mismatched resume report = %#v, error = %v", mismatch, err)
	}

	resumed, err := RunReencrypt(ctx, ReencryptOptions{
		Database: database, OldCipher: oldCipher, NewCipher: newCipher,
		BatchSize: 1, Now: time.Now,
	})
	if err != nil || resumed.Status != "success" || !resumed.Resumed || resumed.Counts.Updated != 2 {
		t.Fatalf("resumed report = %#v, error = %v", resumed, err)
	}
	fixture.assertBusinessCiphertexts(t, ctx, oldCipher, newCipher, false)
	fixture.assertNoStagedItems(t, ctx)
}

func TestMySQLReencryptCASFailureRollsBackAllUpdates(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)
	oldCipher := mustTestCipher(t, reencryptTestOldKey)
	newCipher := mustTestCipher(t, reencryptTestNewKey)
	fixture := newReencryptFixture(t, ctx, database, oldCipher, 2, false)
	changedCiphertext, err := oldCipher.Encrypt([]byte("externally changed"), fixture.sites[1].aad)
	if err != nil {
		t.Fatalf("encrypt concurrent value: %v", err)
	}

	report, err := RunReencrypt(ctx, ReencryptOptions{
		Database: database, OldCipher: oldCipher, NewCipher: newCipher,
		BatchSize: 2, Now: time.Now,
		AfterStageBatch: func(int64) error {
			_, updateErr := database.ExecContext(ctx,
				"UPDATE site SET access_token_encrypted = ? WHERE id = ?",
				changedCiphertext, fixture.sites[1].id,
			)
			return updateErr
		},
	})
	if err == nil || report.Error == nil || report.Error.Code != "REENCRYPT_CONCURRENT_CHANGE" {
		t.Fatalf("CAS failure report = %#v, error = %v", report, err)
	}
	fixture.sites[1].ciphertext = changedCiphertext
	fixture.sites[1].plaintext = []byte("externally changed")
	fixture.assertBusinessCiphertexts(t, ctx, oldCipher, newCipher, true)
	fixture.assertNoMaintenanceState(t, ctx)
}

func TestMySQLReencryptRejectsBadCiphertextWithoutWrites(t *testing.T) {
	database, ctx := openReencryptTestDatabase(t)
	oldCipher := mustTestCipher(t, reencryptTestOldKey)
	newCipher := mustTestCipher(t, reencryptTestNewKey)
	fixture := newReencryptFixture(t, ctx, database, oldCipher, 1, false)
	if _, err := database.ExecContext(ctx,
		"UPDATE site SET access_token_encrypted = 'not-a-ciphertext' WHERE id = ?", fixture.sites[0].id,
	); err != nil {
		t.Fatalf("corrupt fixture ciphertext: %v", err)
	}
	fixture.sites[0].ciphertext = "not-a-ciphertext"

	report, err := RunReencrypt(ctx, ReencryptOptions{
		Database: database, OldCipher: oldCipher, NewCipher: newCipher,
		BatchSize: 100, Now: time.Now,
	})
	if err == nil || report.Error == nil || report.Error.Code != "REENCRYPT_CIPHERTEXT_INVALID" {
		t.Fatalf("bad-ciphertext report = %#v, error = %v", report, err)
	}
	fixture.assertNoMaintenanceState(t, ctx)
}

type reencryptFixture struct {
	database     *sql.DB
	jobFloor     int64
	sites        []reencryptFixtureSecret
	setting      *reencryptFixtureSetting
	settingPlain []byte
}

type reencryptFixtureSecret struct {
	id         int64
	aad        string
	plaintext  []byte
	ciphertext string
}

type reencryptFixtureSetting struct {
	id        int64
	key       string
	value     string
	updatedAt int64
}

func newReencryptFixture(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	cipher *common.Cipher,
	siteCount int,
	includeSetting bool,
) *reencryptFixture {
	t.Helper()
	fixture := &reencryptFixture{database: database, sites: make([]reencryptFixtureSecret, 0, siteCount)}
	if err := database.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM encryption_reencrypt_job").Scan(&fixture.jobFloor); err != nil {
		t.Fatalf("read re-encryption job floor: %v", err)
	}
	for index := 0; index < siteCount; index++ {
		result, err := database.ExecContext(ctx, `INSERT INTO site
  (name, base_url, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			fmt.Sprintf("re-encryption fixture %d", index),
			fmt.Sprintf("https://reencrypt-%d-%d.invalid", time.Now().UnixNano(), index),
			time.Now().Unix(), time.Now().Unix(),
		)
		if err != nil {
			t.Fatalf("insert site fixture: %v", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("read site fixture ID: %v", err)
		}
		plaintext := []byte(fmt.Sprintf("site-token-%d", index))
		aad := fmt.Sprintf("site:%d:access_token", id)
		ciphertext, err := cipher.Encrypt(plaintext, aad)
		if err != nil {
			t.Fatalf("encrypt site fixture: %v", err)
		}
		if _, err := database.ExecContext(ctx,
			"UPDATE site SET access_token_encrypted = ? WHERE id = ?", ciphertext, id,
		); err != nil {
			t.Fatalf("store site fixture ciphertext: %v", err)
		}
		fixture.sites = append(fixture.sites, reencryptFixtureSecret{
			id: id, aad: aad, plaintext: plaintext, ciphertext: ciphertext,
		})
	}
	if includeSetting {
		const settingKey = "notification.dingtalk.webhook"
		setting := &reencryptFixtureSetting{key: settingKey}
		if err := database.QueryRowContext(ctx, `SELECT id, setting_value, updated_at
FROM platform_setting WHERE setting_key = ?`, settingKey).Scan(&setting.id, &setting.value, &setting.updatedAt); err != nil {
			t.Fatalf("read setting fixture source: %v", err)
		}
		fixture.settingPlain = []byte("https://oapi.dingtalk.com/robot/send?access_token=test")
		ciphertext, err := cipher.Encrypt(fixture.settingPlain, "setting:"+settingKey)
		if err != nil {
			t.Fatalf("encrypt setting fixture: %v", err)
		}
		if _, err := database.ExecContext(ctx, `UPDATE platform_setting
SET setting_value = ?, updated_at = ? WHERE id = ?`, ciphertext, time.Now().Unix(), setting.id); err != nil {
			t.Fatalf("store setting fixture ciphertext: %v", err)
		}
		fixture.setting = setting
	}
	t.Cleanup(func() { fixture.cleanup(t) })
	return fixture
}

func (fixture *reencryptFixture) assertBusinessCiphertexts(
	t *testing.T,
	ctx context.Context,
	oldCipher *common.Cipher,
	newCipher *common.Cipher,
	wantOld bool,
) {
	t.Helper()
	for _, site := range fixture.sites {
		var ciphertext string
		if err := fixture.database.QueryRowContext(ctx,
			"SELECT access_token_encrypted FROM site WHERE id = ?", site.id,
		).Scan(&ciphertext); err != nil {
			t.Fatalf("read site ciphertext: %v", err)
		}
		assertCipherOwner(t, ciphertext, site.aad, site.plaintext, oldCipher, newCipher, wantOld)
	}
	if fixture.setting != nil {
		var ciphertext string
		if err := fixture.database.QueryRowContext(ctx,
			"SELECT setting_value FROM platform_setting WHERE id = ?", fixture.setting.id,
		).Scan(&ciphertext); err != nil {
			t.Fatalf("read setting ciphertext: %v", err)
		}
		assertCipherOwner(
			t, ciphertext, "setting:"+fixture.setting.key, fixture.settingPlain,
			oldCipher, newCipher, wantOld,
		)
	}
}

func assertCipherOwner(
	t *testing.T,
	ciphertext string,
	aad string,
	plaintext []byte,
	oldCipher *common.Cipher,
	newCipher *common.Cipher,
	wantOld bool,
) {
	t.Helper()
	owner, other := oldCipher, newCipher
	if !wantOld {
		owner, other = newCipher, oldCipher
	}
	decrypted, err := owner.Decrypt(ciphertext, aad)
	if err != nil || string(decrypted) != string(plaintext) {
		t.Fatalf("owner decrypt = %q, %v", decrypted, err)
	}
	if _, err := other.Decrypt(ciphertext, aad); err == nil {
		t.Fatal("ciphertext was also accepted by the other key")
	}
}

func (fixture *reencryptFixture) assertNoMaintenanceState(t *testing.T, ctx context.Context) {
	t.Helper()
	fixture.assertNoStagedItems(t, ctx)
	var active int
	if err := fixture.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM encryption_reencrypt_job
WHERE id > ? AND active_key IS NOT NULL`, fixture.jobFloor).Scan(&active); err != nil {
		t.Fatalf("count active jobs: %v", err)
	}
	if active != 0 {
		t.Fatalf("active re-encryption jobs = %d", active)
	}
}

func (fixture *reencryptFixture) assertNoStagedItems(t *testing.T, ctx context.Context) {
	t.Helper()
	var staged int
	if err := fixture.database.QueryRowContext(ctx, `SELECT COUNT(*) FROM encryption_reencrypt_item
WHERE job_id > ?`, fixture.jobFloor).Scan(&staged); err != nil {
		t.Fatalf("count staged items: %v", err)
	}
	if staged != 0 {
		t.Fatalf("staged re-encryption items = %d", staged)
	}
}

func (fixture *reencryptFixture) cleanup(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = fixture.database.ExecContext(ctx, "DELETE FROM encryption_reencrypt_item WHERE job_id > ?", fixture.jobFloor)
	_, _ = fixture.database.ExecContext(ctx, "DELETE FROM encryption_reencrypt_job WHERE id > ?", fixture.jobFloor)
	if fixture.setting != nil {
		_, _ = fixture.database.ExecContext(ctx, `UPDATE platform_setting
SET setting_value = ?, updated_at = ? WHERE id = ?`, fixture.setting.value, fixture.setting.updatedAt, fixture.setting.id)
	}
	for _, site := range fixture.sites {
		if _, err := fixture.database.ExecContext(ctx, "DELETE FROM site WHERE id = ?", site.id); err != nil {
			t.Errorf("delete site fixture %d: %v", site.id, err)
		}
	}
}

func openReencryptTestDatabase(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 8, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		t.Fatalf("seed defaults: %v", err)
	}
	return database.SQL, ctx
}

func mustTestCipher(t *testing.T, key []byte) *common.Cipher {
	t.Helper()
	cipher, err := common.NewCipher(key)
	if err != nil {
		t.Fatalf("create test cipher: %v", err)
	}
	return cipher
}
