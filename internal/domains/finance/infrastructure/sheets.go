package infrastructure

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ai-assistant/internal/domains/finance/domain"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const spreadsheetTimezone = "Asia/Jakarta"

type DisabledSyncer struct{}

type Syncer interface {
	Sync(context.Context, domain.Record) (domain.SyncStatus, error)
}

func (DisabledSyncer) Sync(context.Context, domain.Record) (domain.SyncStatus, error) {
	return domain.SyncDisabled, nil
}

type GoogleSheetsSyncer struct {
	service       *sheets.Service
	spreadsheetID string
	location      *time.Location
}

func NewGoogleSheetsSyncer(ctx context.Context, spreadsheetID, encodedCredentials string) (Syncer, error) {
	spreadsheetID = strings.TrimSpace(spreadsheetID)
	encodedCredentials = strings.TrimSpace(encodedCredentials)
	if spreadsheetID == "" && encodedCredentials == "" {
		return DisabledSyncer{}, nil
	}
	if spreadsheetID == "" || encodedCredentials == "" {
		return nil, errors.New("GOOGLE_SHEETS_SPREADSHEET_ID and GOOGLE_SERVICE_ACCOUNT_JSON_BASE64 must both be set")
	}
	credentialsJSON, err := base64.StdEncoding.DecodeString(encodedCredentials)
	if err != nil {
		return nil, fmt.Errorf("decode Google service account credentials: %w", err)
	}
	credentials, err := google.CredentialsFromJSON(ctx, credentialsJSON, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("parse Google service account credentials: %w", err)
	}
	service, err := sheets.NewService(ctx, option.WithTokenSource(credentials.TokenSource))
	if err != nil {
		return nil, fmt.Errorf("create Google Sheets service: %w", err)
	}
	location, err := time.LoadLocation(spreadsheetTimezone)
	if err != nil {
		return nil, fmt.Errorf("load spreadsheet timezone: %w", err)
	}
	return &GoogleSheetsSyncer{service: service, spreadsheetID: spreadsheetID, location: location}, nil
}

func (s *GoogleSheetsSyncer) Sync(ctx context.Context, input domain.Record) (domain.SyncStatus, error) {
	title := indonesianMonth(input.CreatedAt.In(s.location).Month())
	sheetID, err := s.sheetID(ctx, title)
	if err != nil {
		return domain.SyncPending, err
	}
	values, err := s.service.Spreadsheets.Values.Get(s.spreadsheetID, sheetRange(title, "A:Z")).Context(ctx).Do()
	if err != nil {
		return domain.SyncPending, fmt.Errorf("read spreadsheet layout: %w", err)
	}
	layout, err := spreadsheetLayoutFromValues(values.Values)
	if err != nil {
		return domain.SyncPending, err
	}
	input.CreatedAt = input.CreatedAt.In(s.location)
	row, err := spreadsheetRow(layout.headers, layout.nextRecordNumber, input)
	if err != nil {
		return domain.SyncPending, err
	}
	writeRow := layout.nextWriteRow
	if layout.totalRow > 0 {
		_, err = s.service.Spreadsheets.BatchUpdate(s.spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{{InsertDimension: &sheets.InsertDimensionRequest{Range: &sheets.DimensionRange{SheetId: sheetID, Dimension: "ROWS", StartIndex: int64(layout.totalRow - 1), EndIndex: int64(layout.totalRow)}, InheritFromBefore: true}}}}).Context(ctx).Do()
		if err != nil {
			return domain.SyncPending, fmt.Errorf("insert spreadsheet row: %w", err)
		}
		writeRow = layout.totalRow
	}
	_, err = s.service.Spreadsheets.Values.Update(s.spreadsheetID, sheetRange(title, fmt.Sprintf("A%d", writeRow)), &sheets.ValueRange{Values: [][]any{row}}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return domain.SyncPending, fmt.Errorf("write spreadsheet row: %w", err)
	}
	return domain.SyncSynced, nil
}

func (s *GoogleSheetsSyncer) sheetID(ctx context.Context, title string) (int64, error) {
	spreadsheet, err := s.service.Spreadsheets.Get(s.spreadsheetID).Fields("sheets.properties(sheetId,title)").Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("read spreadsheet sheets: %w", err)
	}
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == title {
			return sheet.Properties.SheetId, nil
		}
	}
	return 0, fmt.Errorf("spreadsheet does not contain %q sheet", title)
}

type spreadsheetLayout struct {
	headers                                  []string
	totalRow, nextRecordNumber, nextWriteRow int
}

func spreadsheetLayoutFromValues(values [][]any) (spreadsheetLayout, error) {
	if len(values) == 0 {
		return spreadsheetLayout{}, errors.New("spreadsheet sheet is empty")
	}
	headers := transactionHeaders(values[0])
	if err := validateSpreadsheetHeaders(headers); err != nil {
		return spreadsheetLayout{}, err
	}
	layout := spreadsheetLayout{headers: headers, nextRecordNumber: 1, nextWriteRow: len(values) + 1}
	for index, row := range values[1:] {
		if len(row) == 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(row[0])), "Total") {
			layout.totalRow = index + 2
			break
		}
		if number, ok := spreadsheetRecordNumber(row[0]); ok && number >= layout.nextRecordNumber {
			layout.nextRecordNumber = number + 1
		}
	}
	return layout, nil
}

func transactionHeaders(row []any) []string {
	var headers []string
	for _, cell := range row {
		header := strings.TrimSpace(fmt.Sprint(cell))
		if header == "" {
			break
		}
		headers = append(headers, header)
	}
	return headers
}

func validateSpreadsheetHeaders(headers []string) error {
	required := map[string]bool{"no": false, "tanggal": false, "nama pengeluaran": false, "jumlah": false, "harga": false}
	for _, header := range headers {
		if _, ok := required[normalizedHeader(header)]; ok {
			required[normalizedHeader(header)] = true
		}
	}
	for header, present := range required {
		if !present {
			return fmt.Errorf("spreadsheet transaction header %q is required", header)
		}
	}
	return nil
}

func spreadsheetRow(headers []string, number int, input domain.Record) ([]any, error) {
	row := make([]any, len(headers))
	for index, header := range headers {
		switch normalizedHeader(header) {
		case "no":
			row[index] = number
		case "tanggal":
			row[index] = input.CreatedAt.Format("02/01/2006")
		case "nama pengeluaran":
			row[index] = input.Description
		case "jumlah":
			row[index] = 1
		case "kategori":
			row[index] = input.Category
		case "harga":
			row[index] = input.Amount
		default:
			return nil, fmt.Errorf("unsupported spreadsheet transaction header %q", header)
		}
	}
	return row, nil
}

func spreadsheetRecordNumber(value any) (int, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
	return int(number), err == nil && number >= 1 && number == float64(int(number))
}
func normalizedHeader(header string) string {
	return strings.ToLower(strings.Join(strings.Fields(header), " "))
}
func sheetRange(title, cells string) string {
	return "'" + strings.ReplaceAll(title, "'", "''") + "'!" + cells
}
func indonesianMonth(month time.Month) string {
	return []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}[month]
}
