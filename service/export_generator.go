package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"

	"new-api-pilot/dto"
)

const (
	exportXLSXDataRowsPerSheet = 1_000_000
	exportTimezone             = "Asia/Shanghai"
)

var (
	ErrExportFileTooLarge = errors.New("export file exceeds the configured limit")
	ErrExportWrite        = errors.New("export file could not be written")
)

type ExportFileTooLargeError struct {
	ObservedBytes int64
	LimitBytes    int64
}

func (err *ExportFileTooLargeError) Error() string { return ErrExportFileTooLarge.Error() }
func (err *ExportFileTooLargeError) Unwrap() error { return ErrExportFileTooLarge }

var exportColumns = []string{
	"scope_type", "scope_id", "scope_name", "site_id/site_name",
	"bucket_start", "bucket_end", "timezone",
	"request_count", "quota", "token_used", "active_users",
	"quota_per_unit", "usd_exchange_rate", "rate_source", "amount_usd", "amount_cny",
	"data_status", "is_final", "as_of", "data_snapshot_at", "exported_at",
}

type ExportRateSnapshot struct {
	Sites []ExportRateSnapshotSite `json:"sites"`
}

type ExportRateSnapshotSite struct {
	SiteID          string  `json:"site_id"`
	SiteName        string  `json:"site_name"`
	QuotaPerUnit    *string `json:"quota_per_unit"`
	USDExchangeRate *string `json:"usd_exchange_rate"`
	RateSource      string  `json:"rate_source"`
	RateUpdatedAt   *int64  `json:"rate_updated_at"`
}

type ExportGenerateOptions struct {
	Iterator       *StatisticsExportIterator
	Format         string
	TemporaryPath  string
	Rates          ExportRateSnapshot
	DataSnapshotAt int64
	ExportedAt     int64
	MaxFileBytes   int64
	MinFreeBytes   int64
	DiskFree       ExportDiskFreeFunc
	OnPage         func(context.Context, int, int64) error
	SheetRowLimit  int
}

type ExportGenerateResult struct {
	FileSize int64
	RowCount int64
}

func NewExportRateSnapshot(sites []dto.SiteQuotaBreakdown) ExportRateSnapshot {
	result := ExportRateSnapshot{Sites: make([]ExportRateSnapshotSite, 0, len(sites))}
	for _, site := range sites {
		result.Sites = append(result.Sites, ExportRateSnapshotSite{
			SiteID: site.SiteID, SiteName: site.SiteName, QuotaPerUnit: site.QuotaPerUnit,
			USDExchangeRate: site.USDExchangeRate, RateSource: site.RateSource, RateUpdatedAt: site.RateUpdatedAt,
		})
	}
	sort.Slice(result.Sites, func(left, right int) bool { return result.Sites[left].SiteID < result.Sites[right].SiteID })
	return result
}

func ParseExportRateSnapshot(raw json.RawMessage) (ExportRateSnapshot, error) {
	var result ExportRateSnapshot
	if len(raw) == 0 || json.Unmarshal(raw, &result) != nil {
		return ExportRateSnapshot{}, ErrExportContract
	}
	seen := make(map[string]struct{}, len(result.Sites))
	for _, site := range result.Sites {
		if _, exists := seen[site.SiteID]; exists || !canonicalExportIDString(site.SiteID) ||
			!validExportRate(site.QuotaPerUnit) || !validExportRate(site.USDExchangeRate) ||
			(site.RateSource != "site" && site.RateSource != "fallback" && site.RateSource != "unavailable") {
			return ExportRateSnapshot{}, ErrExportContract
		}
		seen[site.SiteID] = struct{}{}
	}
	return result, nil
}

func GenerateExportFile(ctx context.Context, options ExportGenerateOptions) (ExportGenerateResult, error) {
	if options.Iterator == nil || options.TemporaryPath == "" || options.DataSnapshotAt <= 0 || options.ExportedAt <= 0 ||
		options.MaxFileBytes <= 0 || options.MinFreeBytes <= 0 ||
		(options.Format != dto.ExportFormatCSV && options.Format != dto.ExportFormatXLSX) {
		return ExportGenerateResult{}, ErrExportInvalid
	}
	if options.DiskFree == nil {
		options.DiskFree = availableExportDiskBytes
	}
	if err := checkExportDisk(options); err != nil {
		return ExportGenerateResult{}, err
	}
	file, err := os.OpenFile(options.TemporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	var writer exportRowWriter
	if options.Format == dto.ExportFormatCSV {
		writer, err = newCSVExportWriter(file, options.MaxFileBytes)
	} else {
		limit := options.SheetRowLimit
		if limit <= 0 {
			limit = exportXLSXDataRowsPerSheet
		}
		writer, err = newXLSXExportWriter(file, options.TemporaryPath, options.MaxFileBytes, limit)
	}
	if err != nil {
		return ExportGenerateResult{}, err
	}
	if err := writer.WriteHeader(exportColumns); err != nil {
		return ExportGenerateResult{}, err
	}
	rateBySite := make(map[string]ExportRateSnapshotSite, len(options.Rates.Sites))
	for _, rate := range options.Rates.Sites {
		rateBySite[rate.SiteID] = rate
	}
	var rowCount int64
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return ExportGenerateResult{}, err
		}
		response, done, readErr := options.Iterator.Next(ctx)
		if readErr != nil {
			return ExportGenerateResult{}, readErr
		}
		if done {
			break
		}
		for _, item := range response.Breakdown.Items {
			rows, rowErr := exportRows(item, rateBySite, options.DataSnapshotAt, options.ExportedAt)
			if rowErr != nil {
				return ExportGenerateResult{}, rowErr
			}
			for _, row := range rows {
				if err := writer.WriteRow(row); err != nil {
					return ExportGenerateResult{}, err
				}
				rowCount++
			}
		}
		if err := checkExportDisk(options); err != nil {
			return ExportGenerateResult{}, err
		}
		if options.Format == dto.ExportFormatCSV {
			if info, statErr := file.Stat(); statErr != nil {
				return ExportGenerateResult{}, errors.Join(ErrExportWrite, statErr)
			} else if info.Size() > options.MaxFileBytes {
				return ExportGenerateResult{}, &ExportFileTooLargeError{
					ObservedBytes: info.Size(),
					LimitBytes:    options.MaxFileBytes,
				}
			}
		}
		if options.OnPage != nil {
			if err := options.OnPage(ctx, page, rowCount); err != nil {
				return ExportGenerateResult{}, err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return ExportGenerateResult{}, err
	}
	if err := file.Sync(); err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	if err := file.Close(); err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	closed = true
	info, err := os.Stat(options.TemporaryPath)
	if err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	if info.Size() > options.MaxFileBytes {
		return ExportGenerateResult{}, &ExportFileTooLargeError{
			ObservedBytes: info.Size(),
			LimitBytes:    options.MaxFileBytes,
		}
	}
	return ExportGenerateResult{FileSize: info.Size(), RowCount: rowCount}, nil
}

type exportRowWriter interface {
	WriteHeader([]string) error
	WriteRow([]string) error
	Close() error
}

type csvExportWriter struct {
	writer  *csv.Writer
	limited *exportLimitedWriter
}

func newCSVExportWriter(destination io.Writer, limit int64) (*csvExportWriter, error) {
	limited := &exportLimitedWriter{destination: destination, limit: limit}
	if _, err := limited.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
		return nil, errors.Join(ErrExportWrite, err)
	}
	return &csvExportWriter{writer: csv.NewWriter(limited), limited: limited}, nil
}

func (writer *csvExportWriter) WriteHeader(values []string) error { return writer.write(values) }
func (writer *csvExportWriter) WriteRow(values []string) error    { return writer.write(values) }

func (writer *csvExportWriter) write(values []string) error {
	safe := make([]string, len(values))
	for index, value := range values {
		safe[index] = sanitizeCSVFormula(value)
	}
	if err := writer.writer.Write(safe); err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	writer.writer.Flush()
	if err := writer.writer.Error(); err != nil {
		if errors.Is(err, ErrExportFileTooLarge) {
			return err
		}
		return errors.Join(ErrExportWrite, err)
	}
	return nil
}

func (writer *csvExportWriter) Close() error {
	writer.writer.Flush()
	if err := writer.writer.Error(); err != nil {
		if errors.Is(err, ErrExportFileTooLarge) {
			return err
		}
		return errors.Join(ErrExportWrite, err)
	}
	return nil
}

type xlsxExportWriter struct {
	book        *excelize.File
	destination io.Writer
	limited     *exportLimitedWriter
	stream      *excelize.StreamWriter
	style       int
	sheet       int
	row         int
	rowLimit    int
}

func newXLSXExportWriter(destination io.Writer, temporaryPath string, limit int64, rowLimit int) (*xlsxExportWriter, error) {
	book := excelize.NewFile(excelize.Options{TmpDir: filepathDir(temporaryPath)})
	if err := book.SetSheetName("Sheet1", "Data"); err != nil {
		_ = book.Close()
		return nil, errors.Join(ErrExportWrite, err)
	}
	style, err := book.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		_ = book.Close()
		return nil, errors.Join(ErrExportWrite, err)
	}
	stream, err := book.NewStreamWriter("Data")
	if err != nil {
		_ = book.Close()
		return nil, errors.Join(ErrExportWrite, err)
	}
	return &xlsxExportWriter{
		book: book, destination: destination,
		limited: &exportLimitedWriter{destination: destination, limit: limit},
		stream:  stream, style: style, sheet: 1, row: 0, rowLimit: rowLimit,
	}, nil
}

func (writer *xlsxExportWriter) WriteHeader(values []string) error {
	return writer.write(values, false)
}
func (writer *xlsxExportWriter) WriteRow(values []string) error { return writer.write(values, true) }

func (writer *xlsxExportWriter) write(values []string, data bool) error {
	if data && writer.row >= writer.rowLimit+1 {
		if err := writer.nextSheet(); err != nil {
			return err
		}
		if err := writer.write(exportColumns, false); err != nil {
			return err
		}
	}
	writer.row++
	axis, err := excelize.CoordinatesToCellName(1, writer.row)
	if err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	cells := make([]any, len(values))
	for index, value := range values {
		cells[index] = excelize.Cell{StyleID: writer.style, Value: value}
	}
	if err := writer.stream.SetRow(axis, cells); err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	return nil
}

func (writer *xlsxExportWriter) nextSheet() error {
	if err := writer.stream.Flush(); err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	writer.sheet++
	name := "Data-" + strconv.Itoa(writer.sheet)
	if _, err := writer.book.NewSheet(name); err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	stream, err := writer.book.NewStreamWriter(name)
	if err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	writer.stream = stream
	writer.row = 0
	return nil
}

func (writer *xlsxExportWriter) Close() error {
	defer func() { _ = writer.book.Close() }()
	if err := writer.stream.Flush(); err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	if err := writer.book.Write(writer.limited); err != nil {
		if errors.Is(err, ErrExportFileTooLarge) {
			return err
		}
		return errors.Join(ErrExportWrite, err)
	}
	return nil
}

type exportLimitedWriter struct {
	destination io.Writer
	limit       int64
	written     int64
}

func (writer *exportLimitedWriter) Write(data []byte) (int, error) {
	if writer.written+int64(len(data)) > writer.limit {
		return 0, &ExportFileTooLargeError{
			ObservedBytes: writer.written + int64(len(data)),
			LimitBytes:    writer.limit,
		}
	}
	written, err := writer.destination.Write(data)
	writer.written += int64(written)
	return written, err
}

func exportRows(
	item dto.StatisticsBreakdownItem,
	rates map[string]ExportRateSnapshotSite,
	dataSnapshotAt, exportedAt int64,
) ([][]string, error) {
	base, scope, err := exportBreakdownBase(item)
	if err != nil {
		return nil, err
	}
	sites := base.SiteBreakdown
	if len(sites) == 0 {
		site := dto.SiteQuotaBreakdown{Quota: base.Quota, DataStatus: base.DataStatus}
		if base.SiteID != nil {
			site.SiteID = *base.SiteID
		}
		if base.SiteName != nil {
			site.SiteName = *base.SiteName
		}
		sites = []dto.SiteQuotaBreakdown{site}
	}
	result := make([][]string, 0, len(sites))
	for _, site := range sites {
		rate := rates[site.SiteID]
		quota := exportStringValue(site.Quota)
		quotaPerUnit := exportStringValue(rate.QuotaPerUnit)
		exchangeRate := exportStringValue(rate.USDExchangeRate)
		amountUSD, amountCNY := exportAmounts(quota, quotaPerUnit, exchangeRate)
		siteDisplay := site.SiteID
		if site.SiteName != "" {
			if siteDisplay != "" {
				siteDisplay += " / "
			}
			siteDisplay += site.SiteName
		}
		result = append(result, []string{
			scope, base.DimensionID, base.DimensionName, siteDisplay,
			formatExportTime(base.BucketStart), formatExportTime(base.BucketEnd), exportTimezone,
			exportStringValue(base.RequestCount), quota, exportStringValue(base.TokenUsed), exportStringValue(base.ActiveUsers),
			quotaPerUnit, exchangeRate, rate.RateSource, amountUSD, amountCNY,
			base.DataStatus, strconv.FormatBool(base.IsFinal), formatOptionalExportTime(base.AsOf),
			formatExportTime(dataSnapshotAt), formatExportTime(exportedAt),
		})
	}
	return result, nil
}

func exportBreakdownBase(item dto.StatisticsBreakdownItem) (dto.StatisticsBreakdownBase, string, error) {
	switch value := item.(type) {
	case dto.GlobalStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeGlobal, nil
	case dto.SiteStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeSite, nil
	case dto.CustomerStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeCustomer, nil
	case dto.AccountStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeAccount, nil
	case dto.ModelStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeModel, nil
	case dto.ChannelStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeChannel, nil
	case dto.GroupStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeGroup, nil
	case dto.TokenStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeToken, nil
	case dto.NodeStatisticsBreakdown:
		return value.StatisticsBreakdownBase, dto.StatisticsScopeNode, nil
	default:
		return dto.StatisticsBreakdownBase{}, "", ErrExportContract
	}
}

func exportAmounts(quota, quotaPerUnit, exchangeRate string) (string, string) {
	if quota == "" || quotaPerUnit == "" || exchangeRate == "" {
		return "", ""
	}
	quotaValue, quotaOK := new(big.Rat).SetString(quota)
	unitValue, unitOK := new(big.Rat).SetString(quotaPerUnit)
	exchangeValue, exchangeOK := new(big.Rat).SetString(exchangeRate)
	if !quotaOK || !unitOK || !exchangeOK || unitValue.Sign() <= 0 || exchangeValue.Sign() <= 0 {
		return "", ""
	}
	usd := new(big.Rat).Quo(quotaValue, unitValue)
	cny := new(big.Rat).Mul(usd, exchangeValue)
	return usd.FloatString(6), cny.FloatString(6)
}

func sanitizeCSVFormula(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

func checkExportDisk(options ExportGenerateOptions) error {
	free, err := options.DiskFree(filepathDir(options.TemporaryPath))
	if err != nil {
		return errors.Join(ErrExportWrite, err)
	}
	if free < uint64(options.MinFreeBytes) {
		return &ExportDiskLowError{FreeBytes: free, ThresholdBytes: options.MinFreeBytes}
	}
	return nil
}

func formatExportTime(timestamp int64) string {
	return time.Unix(timestamp, 0).In(time.FixedZone(exportTimezone, 8*60*60)).Format("2006-01-02 15:04:05")
}

func formatOptionalExportTime(timestamp *int64) string {
	if timestamp == nil {
		return ""
	}
	return formatExportTime(*timestamp)
}

func exportStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func filepathDir(path string) string {
	index := strings.LastIndexAny(path, `/\`)
	if index < 0 {
		return "."
	}
	return path[:index]
}

func canonicalExportIDString(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func validExportRate(value *string) bool {
	if value == nil {
		return true
	}
	parsed, ok := new(big.Rat).SetString(*value)
	return ok && parsed.Sign() > 0
}
