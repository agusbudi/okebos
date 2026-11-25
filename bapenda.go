package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"
)

// Simulator BAPENDA: menghasilkan transaksi realisasi pendapatan tiruan.

type CashflowItem struct {
	Tanggal      string `json:"tanggal"`
	KodeRekening string `json:"kode_rekening"`
	Uraian       string `json:"uraian"`
	Jumlah       int64  `json:"jumlah"`
}

type CashflowBatch struct {
	BatchID string         `json:"batch_id"`
	Data    []CashflowItem `json:"data"`
}

var rekening = []struct{ kode, uraian string }{
	{"4.1.01.01", "Pajak Hotel"},
	{"4.1.01.02", "Pajak Restoran"},
	{"4.1.01.03", "Pajak Hiburan"},
	{"4.1.01.04", "Pajak Reklame"},
	{"4.1.01.07", "Pajak Parkir"},
	{"4.1.01.11", "PBB-P2"},
	{"4.1.01.12", "BPHTB"},
	{"4.1.02.01", "Retribusi Pasar"},
}

func generateBatch() CashflowBatch {
	now := time.Now()
	b := CashflowBatch{BatchID: now.Format("20060102-150405.000")}
	for i, n := 0, 5+rand.IntN(6); i < n; i++ {
		rk := rekening[rand.IntN(len(rekening))]
		b.Data = append(b.Data, CashflowItem{
			Tanggal:      now.AddDate(0, 0, -rand.IntN(30)).Format("2006-01-02"),
			KodeRekening: rk.kode,
			Uraian:       "Realisasi " + rk.uraian,
			Jumlah:       int64(5_000+rand.IntN(245_000)) * 1000, // Rp5 jt - Rp250 jt
		})
	}
	return b
}

// apiCashflow adalah endpoint "BAPENDA" yang ditarik oleh sistem manajemen.
func (a *App) apiCashflow(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Bapenda-Key") != a.bapendaKey {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(generateBatch())
}

// pullBapenda menarik data dari endpoint BAPENDA lalu menyimpannya.
func (a *App) pullBapenda(u *User) (CashflowBatch, error) {
	var batch CashflowBatch
	req, _ := http.NewRequest("GET", a.selfURL+"/api/bapenda/cashflow", nil)
	req.Header.Set("X-Bapenda-Key", a.bapendaKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return batch, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return batch, fmt.Errorf("BAPENDA membalas status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&batch); err != nil {
		return batch, err
	}

	tx, err := a.db.Begin()
	if err != nil {
		return batch, err
	}
	defer tx.Rollback()
	for _, it := range batch.Data {
		if _, err := tx.Exec("INSERT INTO bapenda_cashflow(tanggal,kode_rekening,uraian,jumlah,batch_id) VALUES(?,?,?,?,?)",
			it.Tanggal, it.KodeRekening, it.Uraian, it.Jumlah, batch.BatchID); err != nil {
			return batch, err
		}
	}
	if _, err := tx.Exec("INSERT INTO sync_log(waktu,batch_id,jumlah_baris,triggered_by) VALUES(NOW(),?,?,?)",
		batch.BatchID, len(batch.Data), u.ID); err != nil {
		return batch, err
	}
	return batch, tx.Commit()
}
