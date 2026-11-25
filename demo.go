package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand/v2"
	"time"
)

// runDemo menambah data simulasi: batch cashflow BAPENDA, pekerjaan tambahan per OPD,
// riwayat progres, dan alokasi anggaran. Dijalankan dengan `go run . -demo`.
func runDemo(db *sql.DB) error {
	var adminID int
	if err := db.QueryRow("SELECT id FROM users WHERE role='admin' LIMIT 1").Scan(&adminID); err != nil {
		return err
	}

	// 1. Cashflow BAPENDA: 12 batch.
	var rows int
	for i := 0; i < 12; i++ {
		b := generateBatch()
		for _, it := range b.Data {
			if _, err := db.Exec("INSERT INTO bapenda_cashflow(tanggal,kode_rekening,uraian,jumlah,batch_id) VALUES(?,?,?,?,?)",
				it.Tanggal, it.KodeRekening, it.Uraian, it.Jumlah, b.BatchID); err != nil {
				return err
			}
		}
		if _, err := db.Exec("INSERT INTO sync_log(waktu,batch_id,jumlah_baris,triggered_by) VALUES(NOW(),?,?,?)",
			b.BatchID, len(b.Data), adminID); err != nil {
			return err
		}
		rows += len(b.Data)
		time.Sleep(2 * time.Millisecond) // batch_id unik (resolusi milidetik)
	}
	log.Printf("cashflow: 12 batch, %d transaksi", rows)

	// 2. Pekerjaan tambahan per OPD, dengan riwayat progres.
	extra := map[string][]string{
		"PU":     {"Pembangunan Drainase Kawasan Pasar", "Peningkatan Jalan Lingkungan RW 05", "Pembangunan Trotoar Pusat Kota"},
		"DIKBUD": {"Pembangunan Laboratorium SMPN 2", "Pengadaan Meubelair Sekolah Dasar", "Rehabilitasi Gedung Olahraga"},
		"DINKES": {"Pembangunan Ruang IGD Puskesmas Barat", "Pengadaan Ambulans Desa Siaga", "Renovasi Gudang Farmasi"},
	}
	opdRows, err := db.Query("SELECT id,kode FROM opd")
	if err != nil {
		return err
	}
	type opd struct {
		id   int
		kode string
	}
	var opds []opd
	for opdRows.Next() {
		var o opd
		opdRows.Scan(&o.id, &o.kode)
		opds = append(opds, o)
	}
	opdRows.Close()

	today := time.Now()
	nWork := 0
	for _, o := range opds {
		var uid int
		if err := db.QueryRow("SELECT id FROM users WHERE opd_id=? LIMIT 1", o.id).Scan(&uid); err != nil {
			return err
		}
		for _, nama := range extra[o.kode] {
			var exists int
			db.QueryRow("SELECT COUNT(*) FROM pekerjaan WHERE opd_id=? AND nama=?", o.id, nama).Scan(&exists)
			if exists > 0 {
				continue
			}
			nilai := int64(200+rand.IntN(1800)) * 1_000_000
			mulai := today.AddDate(0, -(1 + rand.IntN(8)), 0)
			selesai := mulai.AddDate(0, 4+rand.IntN(6), 0)
			target := []int{15, 35, 55, 75, 100}[rand.IntN(5)]
			status := "berjalan"
			if target == 100 {
				status = "selesai"
			}
			res, err := db.Exec("INSERT INTO pekerjaan(opd_id,nama,nilai_kontrak,tgl_mulai,tgl_selesai,status) VALUES(?,?,?,?,?,?)",
				o.id, nama, nilai, mulai.Format("2006-01-02"), selesai.Format("2006-01-02"), status)
			if err != nil {
				return err
			}
			pid, _ := res.LastInsertId()
			// Progres naik bertahap sampai target.
			cur, minggu := 0, 8
			for cur < target {
				cur = min(target, cur+5+rand.IntN(20))
				tgl := today.AddDate(0, 0, -7*minggu).Format("2006-01-02")
				minggu = max(0, minggu-1)
				if _, err := db.Exec("INSERT INTO progress_pekerjaan(pekerjaan_id,persen,keterangan,tanggal,created_by) VALUES(?,?,?,?,?)",
					pid, cur, fmt.Sprintf("Progres fisik %d%%", cur), tgl, uid); err != nil {
					return err
				}
			}
			nWork++
		}
	}
	log.Printf("pekerjaan baru: %d", nWork)

	// 3. Alokasi: tiap pekerjaan berjalan menerima 30-70% nilai kontrak dari cashflow yang masih bersaldo.
	works, err := db.Query(`SELECT p.id,p.nilai_kontrak-COALESCE((SELECT SUM(nominal) FROM budget_allocations WHERE pekerjaan_id=p.id),0)
		FROM pekerjaan p WHERE p.status='berjalan'`)
	if err != nil {
		return err
	}
	type need struct {
		id   int
		sisa int64
	}
	var needs []need
	for works.Next() {
		var n need
		works.Scan(&n.id, &n.sisa)
		needs = append(needs, n)
	}
	works.Close()

	cash, err := db.Query(`SELECT c.id,c.jumlah-COALESCE((SELECT SUM(nominal) FROM budget_allocations WHERE cashflow_id=c.id),0)
		FROM bapenda_cashflow c ORDER BY c.id`)
	if err != nil {
		return err
	}
	type pool struct {
		id    int
		saldo int64
	}
	var pools []pool
	for cash.Next() {
		var p pool
		cash.Scan(&p.id, &p.saldo)
		if p.saldo > 0 {
			pools = append(pools, p)
		}
	}
	cash.Close()

	nAlok := 0
	pi := 0
	for _, n := range needs {
		want := n.sisa * int64(30+rand.IntN(41)) / 100
		for want > 0 && pi < len(pools) {
			take := min(want, pools[pi].saldo)
			if _, err := db.Exec("INSERT INTO budget_allocations(pekerjaan_id,cashflow_id,nominal,dibuat_oleh,dibuat_pada) VALUES(?,?,?,?,NOW())",
				n.id, pools[pi].id, take, adminID); err != nil {
				return err
			}
			nAlok++
			want -= take
			pools[pi].saldo -= take
			if pools[pi].saldo == 0 {
				pi++
			}
		}
	}
	log.Printf("alokasi anggaran: %d entri", nAlok)
	return nil
}
