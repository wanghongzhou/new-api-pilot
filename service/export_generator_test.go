package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"

	"new-api-pilot/common"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func TestCSVExportWriterAddsBOMAndNeutralizesFormulas(t *testing.T) {
	var output bytes.Buffer
	writer, err := newCSVExportWriter(&output, 1<<20)
	if err != nil {
		t.Fatalf("newCSVExportWriter: %v", err)
	}
	values := []string{"plain", "=SUM(A1:A2)", "  +1", "\t-2", "\u3000@cmd", "中文"}
	if err := writer.WriteRow(values); err != nil {
		t.Fatalf("WriteRow: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data := output.Bytes()
	if !bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("CSV does not start with UTF-8 BOM: %x", data[:min(3, len(data))])
	}
	records, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	want := []string{"plain", "'=SUM(A1:A2)", "'  +1", "'\t-2", "'\u3000@cmd", "中文"}
	if len(records) != 1 || !reflect.DeepEqual(records[0], want) {
		t.Fatalf("CSV values = %#v, want %#v", records, want)
	}
}

func TestXLSXExportWriterUsesTextCellsAndSplitsSheets(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "export.xlsx")
	destination, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("create xlsx: %v", err)
	}
	writer, err := newXLSXExportWriter(destination, path, 1<<20, 2)
	if err != nil {
		_ = destination.Close()
		t.Fatalf("newXLSXExportWriter: %v", err)
	}
	if err := writer.WriteHeader(exportColumns); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	for index, value := range []string{"=1+1", "+cmd", "@SUM(A1)"} {
		row := make([]string, len(exportColumns))
		row[0] = dto.StatisticsScopeModel
		row[1] = value
		row[7] = "9007199254740993"
		row[8] = "123456789012345678901234567890"
		row[9] = "9007199254740995"
		row[16] = "complete"
		row[17] = "true"
		if err := writer.WriteRow(row); err != nil {
			t.Fatalf("WriteRow %d: %v", index, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}
	if err := destination.Close(); err != nil {
		t.Fatalf("Close destination: %v", err)
	}
	book, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = book.Close() }()
	if want := []string{"Data", "Data-2"}; !reflect.DeepEqual(book.GetSheetList(), want) {
		t.Fatalf("sheets = %#v, want %#v", book.GetSheetList(), want)
	}
	for column, want := range exportColumns {
		axis, _ := excelize.CoordinatesToCellName(column+1, 1)
		got, getErr := book.GetCellValue("Data", axis)
		if getErr != nil || got != want {
			t.Fatalf("header %s = %q, err=%v, want %q", axis, got, getErr, want)
		}
	}
	checks := []struct {
		sheet string
		axis  string
		value string
	}{
		{sheet: "Data", axis: "B2", value: "=1+1"},
		{sheet: "Data", axis: "B3", value: "+cmd"},
		{sheet: "Data-2", axis: "B2", value: "@SUM(A1)"},
		{sheet: "Data", axis: "H2", value: "9007199254740993"},
	}
	for _, check := range checks {
		got, getErr := book.GetCellValue(check.sheet, check.axis)
		formula, formulaErr := book.GetCellFormula(check.sheet, check.axis)
		if getErr != nil || formulaErr != nil || got != check.value || formula != "" {
			t.Fatalf("%s!%s value=%q formula=%q errors=%v/%v", check.sheet, check.axis, got, formula, getErr, formulaErr)
		}
	}
	dataHeader, err := book.GetCellValue("Data-2", "A1")
	if err != nil || dataHeader != exportColumns[0] {
		t.Fatalf("split sheet header = %q, err=%v", dataHeader, err)
	}
}

func TestExportLimitedWriterReportsObservedSize(t *testing.T) {
	var destination bytes.Buffer
	writer := &exportLimitedWriter{destination: &destination, limit: 5}
	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	_, err := writer.Write([]byte("defg"))
	var tooLarge *ExportFileTooLargeError
	if !errors.As(err, &tooLarge) || !errors.Is(err, ErrExportFileTooLarge) {
		t.Fatalf("overflow error = %v", err)
	}
	if tooLarge.ObservedBytes != 7 || tooLarge.LimitBytes != 5 || destination.String() != "abc" {
		t.Fatalf("overflow = %#v, destination=%q", tooLarge, destination.String())
	}
}

func TestExportRowsPreservePreciseTextNullsAndShanghaiTime(t *testing.T) {
	requestCount := "9007199254740993"
	tokenUsed := "9007199254740995"
	quotaOne := "12345678901234567890"
	quotaTwo := "7"
	asOf := int64(1_767_225_599)
	item := dto.CustomerStatisticsBreakdown{StatisticsBreakdownBase: dto.StatisticsBreakdownBase{
		DimensionID: "42", DimensionName: "客户", BucketStart: 1_767_222_000, BucketEnd: 1_767_225_600,
		RequestCount: &requestCount, TokenUsed: &tokenUsed, DataStatus: "partial", IsFinal: false, AsOf: &asOf,
		SiteBreakdown: []dto.SiteQuotaBreakdown{
			{SiteID: "1", SiteName: "上海", Quota: &quotaOne, DataStatus: "complete"},
			{SiteID: "2", SiteName: "北京", Quota: &quotaTwo, DataStatus: "missing"},
		},
	}}
	unit := "500000"
	exchange := "7.2500000000"
	rows, err := exportRows(item, map[string]ExportRateSnapshotSite{
		"1": {SiteID: "1", QuotaPerUnit: &unit, USDExchangeRate: &exchange, RateSource: "site"},
	}, 1_767_225_610, 1_767_225_620)
	if err != nil {
		t.Fatalf("exportRows: %v", err)
	}
	if len(rows) != 2 || len(rows[0]) != len(exportColumns) || len(rows[1]) != len(exportColumns) {
		t.Fatalf("row shape = %d/%d/%d", len(rows), len(rows[0]), len(rows[1]))
	}
	first := rows[0]
	if first[0] != dto.StatisticsScopeCustomer || first[1] != "42" || first[3] != "1 / 上海" ||
		first[7] != requestCount || first[8] != quotaOne || first[9] != tokenUsed ||
		first[11] != unit || first[12] != exchange || first[13] != "site" ||
		first[14] != "24691357802469.135780" || first[15] != "179012344067901.234405" ||
		first[16] != "partial" || first[17] != "false" {
		t.Fatalf("first row = %#v", first)
	}
	if first[4] != "2026-01-01 07:00:00" || first[5] != "2026-01-01 08:00:00" || first[6] != exportTimezone {
		t.Fatalf("Shanghai timestamps = %#v", first[4:7])
	}
	second := rows[1]
	if second[8] != quotaTwo || second[11] != "" || second[12] != "" || second[14] != "" || second[15] != "" {
		t.Fatalf("missing-rate row = %#v", second)
	}
}

func TestGenerateExportFileChecksDiskPerPageAndUsesPrivateMode(t *testing.T) {
	query := exportTestQuery(t, 1)
	value := "1"
	reader := &fakeExportRowReader{rows: []model.StatisticsExportRow{{
		DimensionID: "global", DimensionName: "全局", BucketStart: query.StartTimestamp,
		BucketEnd: query.EndTimestamp, RequestCount: &value, DataStatus: "complete", IsFinal: true,
		SortKnown: 1, SortNumber: query.StartTimestamp,
	}}}
	request, err := statisticsReadRequest(dto.StatisticsScopeGlobal, query)
	if err != nil {
		t.Fatalf("statisticsReadRequest: %v", err)
	}
	iterator := &StatisticsExportIterator{
		repository: reader, clock: common.SystemClock{}, scope: dto.StatisticsScopeGlobal,
		query: query, request: request, pageSize: StatisticsExportPageSize,
	}
	path := filepath.Join(t.TempDir(), "statistics.csv")
	diskChecks := 0
	pageCallbacks := 0
	result, err := GenerateExportFile(context.Background(), ExportGenerateOptions{
		Iterator: iterator, Format: dto.ExportFormatCSV, TemporaryPath: path,
		DataSnapshotAt: query.EndTimestamp + 1, ExportedAt: query.EndTimestamp + 2,
		MaxFileBytes: 1 << 20, MinFreeBytes: 1,
		DiskFree: func(string) (uint64, error) {
			diskChecks++
			return math.MaxUint64, nil
		},
		OnPage: func(_ context.Context, page int, rows int64) error {
			pageCallbacks++
			if page != 1 || rows != 1 {
				t.Fatalf("page callback = %d/%d", page, rows)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("GenerateExportFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if result.RowCount != 1 || result.FileSize != info.Size() || diskChecks != 2 || pageCallbacks != 1 {
		t.Fatalf("result=%#v size=%d checks=%d callbacks=%d", result, info.Size(), diskChecks, pageCallbacks)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestGenerateExportFileRejectsLowDiskBeforeCreatingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "statistics.csv")
	_, err := GenerateExportFile(context.Background(), ExportGenerateOptions{
		Iterator: &StatisticsExportIterator{}, Format: dto.ExportFormatCSV, TemporaryPath: path,
		DataSnapshotAt: 1, ExportedAt: 1, MaxFileBytes: 100, MinFreeBytes: 10,
		DiskFree: func(string) (uint64, error) { return 9, nil },
	})
	var disk *ExportDiskLowError
	if !errors.As(err, &disk) || disk.FreeBytes != 9 || disk.ThresholdBytes != 10 {
		t.Fatalf("disk error = %#v (%v)", disk, err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temporary file was created: %v", statErr)
	}
}

func TestGenerateEmptyExportStillWritesBOMAndHeader(t *testing.T) {
	query := exportTestQuery(t, 24)
	request, err := statisticsReadRequest(dto.StatisticsScopeModel, query)
	if err != nil {
		t.Fatalf("statisticsReadRequest: %v", err)
	}
	iterator := &StatisticsExportIterator{
		repository: &fakeExportRowReader{}, clock: common.SystemClock{},
		scope: dto.StatisticsScopeModel, query: query, request: request, pageSize: StatisticsExportPageSize,
	}
	path := filepath.Join(t.TempDir(), "empty.csv")
	result, err := GenerateExportFile(context.Background(), ExportGenerateOptions{
		Iterator: iterator, Format: dto.ExportFormatCSV, TemporaryPath: path,
		DataSnapshotAt: query.EndTimestamp + 1, ExportedAt: query.EndTimestamp + 2,
		MaxFileBytes: 1 << 20, MinFreeBytes: 1,
		DiskFree: func(string) (uint64, error) { return math.MaxUint64, nil },
	})
	if err != nil {
		t.Fatalf("GenerateExportFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("empty CSV missing BOM: %x", data[:min(3, len(data))])
	}
	records, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil {
		t.Fatalf("parse empty CSV: %v", err)
	}
	if result.RowCount != 0 || len(records) != 1 || !reflect.DeepEqual(records[0], exportColumns) {
		t.Fatalf("empty export result=%#v records=%#v", result, records)
	}
}
