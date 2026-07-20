package ops

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"new-api-pilot/common"
)

const (
	reencryptLockName        = "new-api-pilot:secrets-reencrypt"
	reencryptActiveKey       = "active"
	reencryptStateStaging    = "staging"
	reencryptStateComplete   = "complete"
	secretRowSiteToken       = "site_access_token"
	secretRowPlatformSetting = "platform_setting_secret"
)

type ReencryptOptions struct {
	Database        *sql.DB
	OldCipher       *common.Cipher
	NewCipher       *common.Cipher
	BatchSize       int
	DryRun          bool
	Now             func() time.Time
	AfterStageBatch func(staged int64) error
}

type secretRow struct {
	Type       string
	ID         int64
	AAD        string
	Ciphertext string
}

type stagedSecret struct {
	RowType       string
	RowID         int64
	AAD           string
	SourceHash    string
	NewCiphertext string
	NeedsUpdate   bool
}

type reencryptJob struct {
	ID            int64
	OldKeyID      string
	NewKeyID      string
	State         string
	InventoryHash string
	TotalItems    int64
	StagedItems   int64
}

type queryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func RunReencrypt(ctx context.Context, options ReencryptOptions) (ReencryptReport, error) {
	oldKeyID, newKeyID := "", ""
	if options.OldCipher != nil {
		oldKeyID = options.OldCipher.KeyID()
	}
	if options.NewCipher != nil {
		newKeyID = options.NewCipher.KeyID()
	}
	report := newReencryptReport(oldKeyID, newKeyID, options.DryRun)
	if options.Database == nil || options.OldCipher == nil || options.NewCipher == nil || oldKeyID == newKeyID {
		return failReencrypt(report, "REENCRYPT_CONFIG_INVALID", "", 0, errors.New("invalid re-encryption dependencies"))
	}
	if options.BatchSize <= 0 || options.BatchSize > 1000 {
		return failReencrypt(report, "REENCRYPT_BATCH_SIZE_INVALID", "", 0, errors.New("batch size must be between 1 and 1000"))
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	connection, err := options.Database.Conn(ctx)
	if err != nil {
		return failReencrypt(report, "REENCRYPT_DATABASE_ERROR", "", 0, err)
	}
	defer func() { _ = connection.Close() }()
	locked, err := acquireMaintenanceLock(ctx, connection, reencryptLockName)
	if err != nil {
		return failReencrypt(report, "REENCRYPT_DATABASE_ERROR", "", 0, err)
	}
	if !locked {
		return failReencrypt(report, "REENCRYPT_LOCKED", "", 0, errors.New("another re-encryption command is active"))
	}
	defer func() {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", reencryptLockName)
	}()

	rows, err := loadSecretRows(ctx, connection)
	if err != nil {
		return failReencrypt(report, "REENCRYPT_DATABASE_ERROR", "", 0, err)
	}
	setInventoryCounts(&report.Counts, rows)
	inventoryHash := hashSecretInventory(rows)

	if options.DryRun {
		for _, row := range rows {
			underNew, _, classifyErr := classifySecret(row, options.OldCipher, options.NewCipher)
			if classifyErr != nil {
				return failReencrypt(report, "REENCRYPT_CIPHERTEXT_INVALID", row.Type, row.ID, classifyErr)
			}
			if underNew {
				report.Counts.NewKey++
			} else {
				report.Counts.OldKey++
			}
		}
		report.Status = "success"
		return report, nil
	}

	job, resumed, err := loadOrCreateReencryptJob(
		ctx, connection, oldKeyID, newKeyID, inventoryHash, int64(len(rows)), options.Now().Unix(),
	)
	if err != nil {
		code := "REENCRYPT_DATABASE_ERROR"
		if errors.Is(err, errActiveReencryptMismatch) {
			code = "REENCRYPT_ACTIVE_JOB_MISMATCH"
		} else if errors.Is(err, errReencryptInventoryChanged) {
			code = "REENCRYPT_INVENTORY_CHANGED"
			if job.ID != 0 {
				_ = cleanupReencryptJob(context.Background(), connection, job.ID)
			}
		}
		return failReencrypt(report, code, "", 0, err)
	}
	report.Resumed = resumed
	existing, err := loadStagedSecrets(ctx, connection, job.ID)
	if err != nil {
		return failReencrypt(report, "REENCRYPT_DATABASE_ERROR", "", 0, err)
	}
	if int64(len(existing)) != job.StagedItems {
		_ = cleanupReencryptJob(context.Background(), connection, job.ID)
		return failReencrypt(
			report, "REENCRYPT_STAGING_INVALID", "", 0,
			errors.New("staged item count does not match the active job"),
		)
	}

	batch := make([]stagedSecret, 0, options.BatchSize)
	for _, row := range rows {
		underNew, plaintext, classifyErr := classifySecret(row, options.OldCipher, options.NewCipher)
		if classifyErr != nil {
			_ = cleanupReencryptJob(context.Background(), connection, job.ID)
			return failReencrypt(report, "REENCRYPT_CIPHERTEXT_INVALID", row.Type, row.ID, classifyErr)
		}
		if underNew {
			report.Counts.NewKey++
		} else {
			report.Counts.OldKey++
		}
		hash := hashCiphertext(row.Ciphertext)
		key := stagedSecretKey(row.Type, row.ID)
		if staged, exists := existing[key]; exists {
			if staged.AAD != row.AAD || staged.SourceHash != hash {
				_ = cleanupReencryptJob(context.Background(), connection, job.ID)
				return failReencrypt(report, "REENCRYPT_CONCURRENT_CHANGE", row.Type, row.ID, errors.New("staged source changed"))
			}
			stagedPlaintext, decryptErr := options.NewCipher.Decrypt(staged.NewCiphertext, staged.AAD)
			if decryptErr != nil || subtle.ConstantTimeCompare(stagedPlaintext, plaintext) != 1 {
				_ = cleanupReencryptJob(context.Background(), connection, job.ID)
				return failReencrypt(report, "REENCRYPT_STAGING_INVALID", row.Type, row.ID, errors.New("staged ciphertext is invalid"))
			}
			continue
		}
		newCiphertext := row.Ciphertext
		if !underNew {
			newCiphertext, err = options.NewCipher.Encrypt(plaintext, row.AAD)
			if err != nil {
				return failReencrypt(report, "REENCRYPT_ENCRYPT_FAILED", row.Type, row.ID, err)
			}
			verified, verifyErr := options.NewCipher.Decrypt(newCiphertext, row.AAD)
			if verifyErr != nil || subtle.ConstantTimeCompare(verified, plaintext) != 1 {
				return failReencrypt(report, "REENCRYPT_ENCRYPT_FAILED", row.Type, row.ID, errors.New("new ciphertext self-check failed"))
			}
		}
		batch = append(batch, stagedSecret{
			RowType: row.Type, RowID: row.ID, AAD: row.AAD, SourceHash: hash,
			NewCiphertext: newCiphertext, NeedsUpdate: !underNew,
		})
		if len(batch) == options.BatchSize {
			if err := stageSecretBatch(ctx, connection, job.ID, batch, options.Now().Unix()); err != nil {
				return failReencrypt(report, "REENCRYPT_DATABASE_ERROR", "", 0, err)
			}
			report.Counts.Staged += int64(len(batch))
			if options.AfterStageBatch != nil {
				if err := options.AfterStageBatch(report.Counts.Staged); err != nil {
					return failReencrypt(report, "REENCRYPT_INTERRUPTED", "", 0, err)
				}
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := stageSecretBatch(ctx, connection, job.ID, batch, options.Now().Unix()); err != nil {
			return failReencrypt(report, "REENCRYPT_DATABASE_ERROR", "", 0, err)
		}
		report.Counts.Staged += int64(len(batch))
		if options.AfterStageBatch != nil {
			if err := options.AfterStageBatch(report.Counts.Staged); err != nil {
				return failReencrypt(report, "REENCRYPT_INTERRUPTED", "", 0, err)
			}
		}
	}
	if resumed {
		report.Counts.Staged = job.StagedItems + report.Counts.Staged
	}

	updated, rowType, rowID, err := applyStagedSecrets(ctx, connection, job.ID, inventoryHash, int64(len(rows)), options.Now().Unix())
	if err != nil {
		if rowType != "" {
			_ = cleanupReencryptJob(context.Background(), connection, job.ID)
			return failReencrypt(report, "REENCRYPT_CONCURRENT_CHANGE", rowType, rowID, err)
		}
		if errors.Is(err, errReencryptInventoryChanged) {
			_ = cleanupReencryptJob(context.Background(), connection, job.ID)
			return failReencrypt(report, "REENCRYPT_INVENTORY_CHANGED", "", 0, err)
		}
		return failReencrypt(report, "REENCRYPT_DATABASE_ERROR", "", 0, err)
	}
	report.Counts.Staged = report.Counts.Total
	report.Counts.Updated = updated
	report.Status = "success"
	return report, nil
}

var (
	errActiveReencryptMismatch   = errors.New("active re-encryption uses different key fingerprints")
	errReencryptInventoryChanged = errors.New("re-encryption inventory changed")
)

func acquireMaintenanceLock(ctx context.Context, connection *sql.Conn, name string) (bool, error) {
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&acquired); err != nil {
		return false, err
	}
	return acquired.Valid && acquired.Int64 == 1, nil
}

func loadSecretRows(ctx context.Context, queryer queryContext) ([]secretRow, error) {
	result := make([]secretRow, 0)
	siteRows, err := queryer.QueryContext(ctx, `SELECT id, access_token_encrypted
FROM site WHERE access_token_encrypted IS NOT NULL AND access_token_encrypted <> '' ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list site secrets: %w", err)
	}
	for siteRows.Next() {
		var row secretRow
		row.Type = secretRowSiteToken
		if err := siteRows.Scan(&row.ID, &row.Ciphertext); err != nil {
			_ = siteRows.Close()
			return nil, err
		}
		row.AAD = "site:" + strconv.FormatInt(row.ID, 10) + ":access_token"
		result = append(result, row)
	}
	if err := siteRows.Close(); err != nil {
		return nil, err
	}
	settingRows, err := queryer.QueryContext(ctx, `SELECT id, setting_key, setting_value
FROM platform_setting WHERE is_secret = 1 AND setting_value <> '' ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list setting secrets: %w", err)
	}
	for settingRows.Next() {
		var row secretRow
		var key string
		row.Type = secretRowPlatformSetting
		if err := settingRows.Scan(&row.ID, &key, &row.Ciphertext); err != nil {
			_ = settingRows.Close()
			return nil, err
		}
		row.AAD = "setting:" + key
		result = append(result, row)
	}
	if err := settingRows.Close(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Type != result[right].Type {
			return result[left].Type < result[right].Type
		}
		return result[left].ID < result[right].ID
	})
	return result, nil
}

func classifySecret(row secretRow, oldCipher, newCipher *common.Cipher) (bool, []byte, error) {
	if plaintext, err := newCipher.Decrypt(row.Ciphertext, row.AAD); err == nil {
		return true, plaintext, nil
	}
	plaintext, err := oldCipher.Decrypt(row.Ciphertext, row.AAD)
	if err != nil {
		return false, nil, common.ErrInvalidCiphertext
	}
	return false, plaintext, nil
}

func hashCiphertext(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func hashSecretInventory(rows []secretRow) string {
	hash := sha256.New()
	for _, row := range rows {
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%s\n", row.Type, row.ID, row.AAD)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func setInventoryCounts(counts *ReencryptCounts, rows []secretRow) {
	counts.Total = int64(len(rows))
	for _, row := range rows {
		switch row.Type {
		case secretRowSiteToken:
			counts.SiteTokens++
		case secretRowPlatformSetting:
			counts.Settings++
		}
	}
}

func stagedSecretKey(rowType string, rowID int64) string {
	return rowType + ":" + strconv.FormatInt(rowID, 10)
}

func loadOrCreateReencryptJob(
	ctx context.Context,
	connection *sql.Conn,
	oldKeyID, newKeyID, inventoryHash string,
	totalItems, now int64,
) (reencryptJob, bool, error) {
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return reencryptJob{}, false, err
	}
	defer func() { _ = transaction.Rollback() }()
	var job reencryptJob
	err = transaction.QueryRowContext(ctx, `SELECT id, old_key_id, new_key_id, state, inventory_hash, total_items, staged_items
FROM encryption_reencrypt_job WHERE active_key = ? FOR UPDATE`, reencryptActiveKey).
		Scan(&job.ID, &job.OldKeyID, &job.NewKeyID, &job.State, &job.InventoryHash, &job.TotalItems, &job.StagedItems)
	resumed := true
	if errors.Is(err, sql.ErrNoRows) {
		resumed = false
		result, insertErr := transaction.ExecContext(ctx, `INSERT INTO encryption_reencrypt_job
  (old_key_id, new_key_id, active_key, state, inventory_hash, total_items, staged_items, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
			oldKeyID, newKeyID, reencryptActiveKey, reencryptStateStaging, inventoryHash, totalItems, now, now)
		if insertErr != nil {
			return reencryptJob{}, false, insertErr
		}
		job.ID, err = result.LastInsertId()
		job.OldKeyID, job.NewKeyID = oldKeyID, newKeyID
		job.State, job.InventoryHash, job.TotalItems = reencryptStateStaging, inventoryHash, totalItems
	} else if err != nil {
		return reencryptJob{}, false, err
	} else {
		if job.OldKeyID != oldKeyID || job.NewKeyID != newKeyID {
			return job, true, errActiveReencryptMismatch
		}
		if job.State != reencryptStateStaging || job.InventoryHash != inventoryHash || job.TotalItems != totalItems {
			return job, true, errReencryptInventoryChanged
		}
	}
	if err := transaction.Commit(); err != nil {
		return reencryptJob{}, resumed, err
	}
	return job, resumed, nil
}

func loadStagedSecrets(ctx context.Context, queryer queryContext, jobID int64) (map[string]stagedSecret, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT row_type, row_id, aad_identity, source_hash, new_ciphertext, needs_update
FROM encryption_reencrypt_item WHERE job_id = ? ORDER BY row_type ASC, row_id ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]stagedSecret)
	for rows.Next() {
		var item stagedSecret
		if err := rows.Scan(
			&item.RowType, &item.RowID, &item.AAD, &item.SourceHash, &item.NewCiphertext, &item.NeedsUpdate,
		); err != nil {
			return nil, err
		}
		result[stagedSecretKey(item.RowType, item.RowID)] = item
	}
	return result, rows.Err()
}

func stageSecretBatch(ctx context.Context, connection *sql.Conn, jobID int64, items []stagedSecret, now int64) error {
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	for _, item := range items {
		if _, err := transaction.ExecContext(ctx, `INSERT INTO encryption_reencrypt_item
  (job_id, row_type, row_id, aad_identity, source_hash, new_ciphertext, needs_update, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			jobID, item.RowType, item.RowID, item.AAD, item.SourceHash, item.NewCiphertext, item.NeedsUpdate, now,
		); err != nil {
			return err
		}
	}
	result, err := transaction.ExecContext(ctx, `UPDATE encryption_reencrypt_job
SET staged_items = staged_items + ?, updated_at = ?
WHERE id = ? AND active_key = ? AND state = ?`, len(items), now, jobID, reencryptActiveKey, reencryptStateStaging)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return errors.New("re-encryption staging job changed")
	}
	return transaction.Commit()
}

func applyStagedSecrets(
	ctx context.Context,
	connection *sql.Conn,
	jobID int64,
	inventoryHash string,
	totalItems, now int64,
) (int64, string, int64, error) {
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", 0, err
	}
	defer func() { _ = transaction.Rollback() }()
	var state, storedInventory string
	var storedTotal, stagedTotal int64
	if err := transaction.QueryRowContext(ctx, `SELECT state, inventory_hash, total_items, staged_items
FROM encryption_reencrypt_job WHERE id = ? AND active_key = ? FOR UPDATE`, jobID, reencryptActiveKey).
		Scan(&state, &storedInventory, &storedTotal, &stagedTotal); err != nil {
		return 0, "", 0, err
	}
	if state != reencryptStateStaging || storedInventory != inventoryHash || storedTotal != totalItems || stagedTotal != totalItems {
		return 0, "", 0, errReencryptInventoryChanged
	}
	currentRows, err := loadSecretRows(ctx, transaction)
	if err != nil {
		return 0, "", 0, err
	}
	if int64(len(currentRows)) != totalItems || hashSecretInventory(currentRows) != inventoryHash {
		return 0, "", 0, errReencryptInventoryChanged
	}
	items, err := loadStagedSecrets(ctx, transaction, jobID)
	if err != nil {
		return 0, "", 0, err
	}
	if int64(len(items)) != totalItems {
		return 0, "", 0, errReencryptInventoryChanged
	}
	var updated int64
	for _, row := range currentRows {
		item, exists := items[stagedSecretKey(row.Type, row.ID)]
		if !exists || item.AAD != row.AAD || item.SourceHash != hashCiphertext(row.Ciphertext) {
			return 0, row.Type, row.ID, errors.New("secret changed after staging")
		}
		if !item.NeedsUpdate {
			continue
		}
		var result sql.Result
		switch row.Type {
		case secretRowSiteToken:
			result, err = transaction.ExecContext(ctx, `UPDATE site SET access_token_encrypted = ?
WHERE id = ? AND SHA2(access_token_encrypted, 256) = ?`, item.NewCiphertext, row.ID, item.SourceHash)
		case secretRowPlatformSetting:
			settingKey := strings.TrimPrefix(item.AAD, "setting:")
			result, err = transaction.ExecContext(ctx, `UPDATE platform_setting SET setting_value = ?
WHERE id = ? AND setting_key = ? AND is_secret = 1 AND SHA2(setting_value, 256) = ?`,
				item.NewCiphertext, row.ID, settingKey, item.SourceHash)
		default:
			return 0, row.Type, row.ID, errors.New("unsupported staged secret type")
		}
		if err != nil {
			return 0, row.Type, row.ID, err
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil || affected != 1 {
			return 0, row.Type, row.ID, errors.New("secret compare-and-swap failed")
		}
		updated++
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM encryption_reencrypt_item WHERE job_id = ?", jobID); err != nil {
		return 0, "", 0, err
	}
	result, err := transaction.ExecContext(ctx, `UPDATE encryption_reencrypt_job
SET active_key = NULL, state = ?, staged_items = total_items, updated_at = ?
WHERE id = ? AND active_key = ?`, reencryptStateComplete, now, jobID, reencryptActiveKey)
	if err != nil {
		return 0, "", 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return 0, "", 0, errors.New("complete re-encryption job compare-and-swap failed")
	}
	if err := transaction.Commit(); err != nil {
		return 0, "", 0, err
	}
	return updated, "", 0, nil
}

func cleanupReencryptJob(ctx context.Context, connection *sql.Conn, jobID int64) error {
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err := transaction.ExecContext(ctx, "DELETE FROM encryption_reencrypt_item WHERE job_id = ?", jobID); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM encryption_reencrypt_job WHERE id = ? AND active_key = ?", jobID, reencryptActiveKey); err != nil {
		return err
	}
	return transaction.Commit()
}

func failReencrypt(
	report ReencryptReport,
	code, rowType string,
	rowID int64,
	cause error,
) (ReencryptReport, error) {
	detail := &OperationError{Code: code, RowType: rowType}
	if rowID > 0 {
		detail.RowID = strconv.FormatInt(rowID, 10)
	}
	report.Status = "failed"
	report.Error = detail
	return report, fmt.Errorf("%s: %w", code, cause)
}
