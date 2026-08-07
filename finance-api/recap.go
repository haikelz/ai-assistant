package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

func (s *server) handleRecap(w http.ResponseWriter, r *http.Request) {
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))
	if phone == "" {
		http.Error(w, "phone is required", http.StatusBadRequest)
		return
	}

	records, err := s.lookupRecords(r.Context(), phone)
	if err != nil {
		http.Error(w, "could not load records", http.StatusInternalServerError)
		return
	}

	file := excelize.NewFile()
	defer file.Close()
	sheet := file.GetSheetName(0)
	rows := [][]any{{"Tanggal", "Tipe", "Kategori", "Deskripsi", "Jumlah"}}
	for _, item := range records {
		rows = append(rows, []any{item.CreatedAt.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02 15:04"), item.Type, item.Category, item.Description, item.Amount})
	}
	if err := file.SetSheetRow(sheet, "A1", &rows[0]); err != nil {
		http.Error(w, "could not create recap", http.StatusInternalServerError)
		return
	}
	for index := 1; index < len(rows); index++ {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil || file.SetSheetRow(sheet, cell, &rows[index]) != nil {
			http.Error(w, "could not create recap", http.StatusInternalServerError)
			return
		}
	}
	if err := file.SetColWidth(sheet, "A", "D", 22); err != nil {
		http.Error(w, "could not create recap", http.StatusInternalServerError)
		return
	}
	if err := file.SetColWidth(sheet, "E", "E", 16); err != nil {
		http.Error(w, "could not create recap", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="finance-recap.xlsx"`)
	if err := file.Write(w); err != nil {
		return
	}
}
