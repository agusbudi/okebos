# Rencana: Simulasi Cashflow & Progress Pekerjaan (okebos)

Stack: Go 1.27 (`net/http`, `html/template`), MySQL (XAMPP).

## 1. Peran dan hak akses

| Peran | Hak akses |
|---|---|
| **OPD** | Input progress pekerjaan. **Hanya melihat milik OPD sendiri** (progress dan anggaran). |
| **Admin / Manajemen** | Menarik data BAPENDA (trigger), menghubungkan anggaran dengan pekerjaan, mengelola user, OPD, dan pekerjaan. Melihat semua data. |
| **Walikota** | Hanya baca. **Melihat semua progress dari semua OPD**. |
| **BAPENDA (simulator)** | Sumber data cashflow tiruan. Bukan user login, berupa generator di sistem. |

Setelah login, semua peran masuk ke **dashboard**. Isinya dibatasi sesuai peran (OPD hanya data OPD-nya).

## 2. Simulasi BAPENDA

- Tombol **"Tarik Data BAPENDA"** di menu Admin menjalankan trigger. Tidak berjalan otomatis.
- Trigger memanggil generator yang membuat transaksi cashflow acak tapi masuk akal: tanggal, kode rekening, uraian, jumlah realisasi pendapatan.
- Hasil masuk ke tabel `bapenda_cashflow`. Setiap penarikan dicatat di `sync_log` (waktu, jumlah baris, user pemicu).
- Opsional: generator diekspos sebagai endpoint `/api/bapenda/cashflow` agar terlihat seperti API eksternal. Admin menarik lewat HTTP client internal.
- Tahap lanjut: **auto-tarik tiap N detik** untuk meniru real time (goroutine + ticker).

## 3. Menghubungkan anggaran dengan pekerjaan

- Sumber dana adalah data cashflow BAPENDA.
- Admin **mengalokasikan** cashflow ke pekerjaan lewat `budget_allocations` (pekerjaan, sumber cashflow, nominal).
- Validasi: total alokasi tidak boleh melebihi saldo cashflow yang tersedia.
- Dashboard menampilkan **anggaran vs progress fisik** per pekerjaan (contoh: serapan 60%, progres fisik 40%).

## 4. Skema database MySQL

```
users(id, username, password_hash, role[admin|opd|walikota], opd_id NULL)
opd(id, kode, nama)
pekerjaan(id, opd_id, nama, nilai_kontrak, tgl_mulai, tgl_selesai, status)
progress_pekerjaan(id, pekerjaan_id, persen, keterangan, tanggal, created_by)
bapenda_cashflow(id, tanggal, kode_rekening, uraian, jumlah, batch_id)
sync_log(id, waktu, jumlah_baris, triggered_by)
budget_allocations(id, pekerjaan_id, cashflow_id/batch_id, nominal, dibuat_oleh)
```

Kolom `status` (berjalan/selesai) dipakai untuk logika "sudah selesai?" pada flowchart walikota.

## 5. Struktur proyek

```
okebos/
  cmd/server/main.go
  internal/
    config/        # DSN, port
    db/            # koneksi + migrasi + seed
    model/
    handler/       # auth, dashboard, pekerjaan, progress, admin, bapenda
    middleware/    # auth, role guard, scoping OPD
    service/       # bapenda_sim, alokasi
  web/templates/   # layout, login, dashboard, pekerjaan, progress, admin_*
  web/static/
  migrations/001_init.sql
```

- Driver: `github.com/go-sql-driver/mysql`. Password: `bcrypt`. Sesi: cookie bertanda tangan (HMAC).
- Chart dashboard: Chart.js (CDN).
- **Keamanan:** pembatasan OPD dilakukan di **query** (`WHERE opd_id = ?` dari sesi), bukan hanya di tampilan, supaya OPD tidak bisa mengintip OPD lain lewat URL.

## 6. Tahapan kerja

1. **Setup**: `go mod init`, koneksi MySQL XAMPP, migrasi, seed (1 admin, 1 walikota, 3 OPD + user, beberapa pekerjaan).
2. **Auth**: login/logout, sesi, middleware peran.
3. **Dashboard** setelah login, per peran: kartu ringkasan dan grafik.
4. **Modul OPD**: daftar pekerjaan miliknya, form tambah progress, riwayat.
5. **Simulator BAPENDA + tombol trigger** di Admin, plus log penarikan.
6. **Alokasi anggaran ke pekerjaan** (Admin) dan tampilan serapan.
7. **Tampilan Walikota**: semua OPD, filter OPD/status, penanda pekerjaan selesai.
8. **Uji manual**: skenario tiap peran, termasuk OPD mencoba membuka data OPD lain (harus ditolak).

## 7. Asumsi / perlu dikonfirmasi

1. DB `okebos`, user `root`, password kosong (default XAMPP).
2. Dashboard Walikota: kartu ringkasan + tabel + grafik (tanpa peta/timeline).
3. Auto-tarik real time (ticker) ditunda ke tahap lanjut.
4. UI berbahasa Indonesia, CSS ringan tanpa framework.
