package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

//go:embed migrations/001_init.sql
var schemaSQL string

// openDB membuat database bila belum ada, lalu menjalankan migrasi.
func openDB(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	name := cfg.DBName
	cfg.DBName = ""
	cfg.MultiStatements = true
	boot, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	_, err = boot.Exec("CREATE DATABASE IF NOT EXISTS `" + name + "` CHARACTER SET utf8mb4")
	boot.Close()
	if err != nil {
		return nil, fmt.Errorf("buat database (MySQL XAMPP sudah jalan?): %w", err)
	}

	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("migrasi: %w", err)
	}
	return db, nil
}

func hash(pw string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h)
}

// seed mengisi data demo hanya jika tabel users masih kosong.
func seed(db *sql.DB) error {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n); err != nil || n > 0 {
		return err
	}
	opds := []struct{ kode, nama string }{
		{"PU", "Dinas Pekerjaan Umum"},
		{"DIKBUD", "Dinas Pendidikan dan Kebudayaan"},
		{"DINKES", "Dinas Kesehatan"},
	}
	opdID := map[string]int64{}
	for _, o := range opds {
		r, err := db.Exec("INSERT INTO opd(kode,nama) VALUES(?,?)", o.kode, o.nama)
		if err != nil {
			return err
		}
		opdID[o.kode], _ = r.LastInsertId()
	}

	users := []struct {
		user, nama, pw, role, opd string
	}{
		{"admin", "Admin Manajemen", "admin123", "admin", ""},
		{"walikota", "Walikota", "walikota123", "walikota", ""},
		{"opd_pu", "Operator Dinas PU", "opd123", "opd", "PU"},
		{"opd_dikbud", "Operator Dinas Pendidikan", "opd123", "opd", "DIKBUD"},
		{"opd_dinkes", "Operator Dinas Kesehatan", "opd123", "opd", "DINKES"},
	}
	userID := map[string]int64{}
	for _, u := range users {
		var opd any
		if u.opd != "" {
			opd = opdID[u.opd]
		}
		r, err := db.Exec("INSERT INTO users(username,nama,password_hash,role,opd_id) VALUES(?,?,?,?,?)",
			u.user, u.nama, hash(u.pw), u.role, opd)
		if err != nil {
			return err
		}
		userID[u.user], _ = r.LastInsertId()
	}

	works := []struct {
		opd, nama string
		nilai     int64
		mulai, sl string
		persen    []int
	}{
		{"PU", "Perbaikan Jalan Protokol Segmen A", 1_500_000_000, "2026-01-05", "2026-09-30", []int{20, 45, 70}},
		{"PU", "Pembangunan Jembatan Kali Putih", 2_400_000_000, "2026-03-01", "2026-12-15", []int{10, 25}},
		{"DIKBUD", "Rehabilitasi Ruang Kelas SDN 1", 800_000_000, "2026-02-01", "2026-07-31", []int{40, 80, 100}},
		{"DIKBUD", "Pengadaan Perpustakaan Digital", 450_000_000, "2026-04-01", "2026-10-31", []int{15}},
		{"DINKES", "Renovasi Puskesmas Kecamatan Timur", 1_100_000_000, "2026-02-15", "2026-11-30", []int{30, 55}},
		{"DINKES", "Pengadaan Alat Kesehatan Posyandu", 350_000_000, "2026-01-10", "2026-05-31", []int{60, 100}},
	}
	for _, w := range works {
		last := w.persen[len(w.persen)-1]
		status := "berjalan"
		if last == 100 {
			status = "selesai"
		}
		r, err := db.Exec("INSERT INTO pekerjaan(opd_id,nama,nilai_kontrak,tgl_mulai,tgl_selesai,status) VALUES(?,?,?,?,?,?)",
			opdID[w.opd], w.nama, w.nilai, w.mulai, w.sl, status)
		if err != nil {
			return err
		}
		pid, _ := r.LastInsertId()
		for i, p := range w.persen {
			tgl := time.Now().AddDate(0, 0, -7*(len(w.persen)-i)).Format("2006-01-02")
			if _, err := db.Exec("INSERT INTO progress_pekerjaan(pekerjaan_id,persen,keterangan,tanggal,created_by) VALUES(?,?,?,?,?)",
				pid, p, "Laporan progres", tgl, userID["opd_"+lower(w.opd)]); err != nil {
				return err
			}
		}
	}
	return nil
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
