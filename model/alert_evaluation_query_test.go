package model

import (
	"context"
	"fmt"
	"testing"

	"new-api-pilot/constant"
)

func TestAlertCollectionSnapshotBoundsFactProofCandidatesPerSite(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(1_752_900_000)
	site := createRunnableSite(t, database, "run-alert-proof-bounded", now)
	baseHour := int64(1_735_689_600) // 2025-01-01 00:00:00 UTC, safely eligible.
	verifiedAt := now
	windows := make([]CollectionWindow, alertValidationProofCandidateLimitPerSite+3)
	for index := range windows {
		windows[index] = CollectionWindow{
			SiteID: site.ID, HourTS: baseHour + int64(index)*3600,
			Status: CollectionWindowStatusComplete, AttributionStatus: UsageAttributionAttributed,
			FactRows: 1, SourceHash: fmt.Sprintf("%064d", index),
			VerifiedAt: &verifiedAt, UpdatedAt: now,
		}
	}
	windows[0].Status = CollectionWindowStatusMissing
	windows[0].LastErrorCode = string(constant.MessageDataValidationMismatch)
	if err := database.GORM.CreateInBatches(&windows, 100).Error; err != nil {
		t.Fatalf("create proof candidates: %v", err)
	}

	rows, err := NewAlertEvaluationRepository(database.GORM).listCollectionWindows(context.Background())
	if err != nil {
		t.Fatalf("list alert collection windows: %v", err)
	}
	got := make([]AlertCollectionEvaluationSnapshot, 0, alertValidationProofCandidateLimitPerSite)
	for _, row := range rows {
		if row.SiteID == site.ID {
			got = append(got, row)
		}
	}
	if len(got) != alertValidationProofCandidateLimitPerSite {
		t.Fatalf("bounded proof candidates = %d, want %d", len(got), alertValidationProofCandidateLimitPerSite)
	}
	if got[0].HourTS != windows[0].HourTS || got[1].HourTS != windows[4].HourTS || got[len(got)-1].HourTS != windows[len(windows)-1].HourTS {
		t.Fatalf("bounded proof range = %d..%d, want %d..%d",
			got[0].HourTS, got[len(got)-1].HourTS, windows[0].HourTS, windows[len(windows)-1].HourTS)
	}
	for _, row := range got {
		if row.ActualFactRows == nil || *row.ActualFactRows != 0 {
			t.Fatalf("proof candidate actual facts = %#v", row.ActualFactRows)
		}
	}
}

func TestAlertCollectionSnapshotBoundsMissingCandidatesPerSite(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(1_752_900_000)
	site := createRunnableSite(t, database, "run-alert-missing-bounded", now)
	baseHour := int64(1_735_689_600)
	windows := make([]CollectionWindow, alertValidationProofCandidateLimitPerSite+8)
	for index := range windows {
		windows[index] = CollectionWindow{
			SiteID: site.ID, HourTS: baseHour + int64(index)*3600,
			Status: CollectionWindowStatusMissing, LastErrorCode: string(constant.MessageDataUpstreamUnavailable), UpdatedAt: now,
		}
	}
	if err := database.GORM.CreateInBatches(&windows, 100).Error; err != nil {
		t.Fatalf("create missing candidates: %v", err)
	}
	rows, err := NewAlertEvaluationRepository(database.GORM).listCollectionWindows(context.Background())
	if err != nil {
		t.Fatalf("list missing alert windows: %v", err)
	}
	count := 0
	minimum, maximum := int64(0), int64(0)
	for _, row := range rows {
		if row.SiteID != site.ID {
			continue
		}
		count++
		if minimum == 0 || row.HourTS < minimum {
			minimum = row.HourTS
		}
		if row.HourTS > maximum {
			maximum = row.HourTS
		}
	}
	if count != alertValidationProofCandidateLimitPerSite || minimum != windows[8].HourTS || maximum != windows[len(windows)-1].HourTS {
		t.Fatalf("bounded missing candidates count=%d range=%d..%d", count, minimum, maximum)
	}
}
