# okebos

Simulasi cashflow dan progress pekerjaan OPD. Aplikasi web ini menghubungkan data realisasi pendapatan (BAPENDA, tiruan) dengan pekerjaan tiap OPD, lalu menampilkan **anggaran vs progress fisik** di dashboard sesuai peran pengguna.

Stack: Go (`net/http`, `html/template`), MySQL (XAMPP), tanpa framework frontend.

## Fitur

- Login dengan sesi cookie bertanda tangan (HMAC) dan password `bcrypt`.
- Tiga peran dengan hak akses berbeda (lihat tabel di bawah).
- Dashboard ringkasan per peran: serapan anggaran dan rata-rata progres.
- Input progress pekerjaan oleh OPD, lengkap dengan riwayat.
- Simulator BAPENDA: tombol **Tarik Data BAPENDA** membuat batch transaksi cashflow acak lewat endpoint internal `/api/bapenda/cashflow`.
- Alokasi cashflow ke pekerjaan, dengan validasi agar total alokasi tidak melebihi saldo.
- Manajemen user dan OPD oleh admin.
- Pembatasan data OPD dilakukan di **query SQL** (`WHERE opd_id = ?` dari sesi), bukan hanya di tampilan.

## Peran

| Peran | Hak akses |
|---|---|
| `opd` | Menambah pekerjaan dan progress. Hanya melihat data OPD sendiri. |
| `admin` | Menarik data BAPENDA, mengalokasikan anggaran, mengelola user/OPD/pekerjaan. Melihat semua data. |
| `walikota` | Hanya baca. Melihat progress semua OPD. |

## Menjalankan

Prasyarat: Go, dan MySQL yang sudah berjalan (mis. XAMPP).

```bash
go run .
```

Buka <http://localhost:8080>. Database `okebos` dibuat otomatis, migrasi dijalankan, dan data awal diisi bila tabel `users` masih kosong.

Untuk menambah data simulasi (batch cashflow, pekerjaan, progres, alokasi):

```bash
go run . -demo
```

### Konfigurasi

| Variabel env | Default | Keterangan |
|---|---|---|
| `ADDR` | `:8080` | Alamat server |
| `DB_DSN` | `root:@tcp(127.0.0.1:3306)/okebos?charset=utf8mb4&loc=Local` | DSN MySQL |

### Akun awal

| Username | Password | Peran |
|---|---|---|
| `admin` | `admin123` | admin |
| `walikota` | `walikota123` | walikota |
| `opd_pu` | `opd123` | opd (Dinas PU) |
| `opd_dikbud` | `opd123` | opd (Dinas Pendidikan) |
| `opd_dinkes` | `opd123` | opd (Dinas Kesehatan) |

Ganti password ini sebelum dipakai di luar lingkungan demo.

## Struktur proyek

```
okebos/
├── main.go                  # entry point, App, template, routing
├── auth.go                  # login/logout, sesi, middleware peran (require)
├── db.go                    # koneksi, migrasi otomatis, seed data awal
├── handlers.go              # dashboard, pekerjaan, progress, BAPENDA, alokasi
├── users.go                 # CRUD user dan OPD (admin)
├── bapenda.go               # simulator BAPENDA + endpoint API + penarikan data
├── demo.go                  # data simulasi (flag -demo)
├── migrations/
│   └── 001_init.sql         # skema database (di-embed ke binary)
├── web/
│   ├── static/              # app.js, style.css
│   └── templates/           # layout, login, dashboard, pekerjaan_*, bapenda,
│                            # alokasi, users, user_edit
├── PLANNING.md              # rencana dan keputusan desain
├── go.mod
└── go.sum
```

Folder `web/` dan `migrations/` di-embed ke binary lewat `go:embed`, jadi aplikasi cukup satu file hasil build.

## Route

| Method | Path | Peran |
|---|---|---|
| GET/POST | `/login`, POST `/logout` | publik |
| GET | `/` (dashboard) | semua peran |
| GET | `/pekerjaan`, `/pekerjaan/{id}` | semua peran (OPD dibatasi) |
| GET/POST | `/pekerjaan/new`, `/pekerjaan` | opd, admin |
| POST | `/pekerjaan/{id}/progress` | opd |
| GET/POST | `/admin/bapenda`, `/admin/bapenda/sync` | admin |
| GET/POST | `/admin/alokasi` | admin |
| GET/POST | `/admin/users`, `/admin/users/{id}[/edit\|/delete]`, `/admin/opd` | admin |
| GET | `/api/bapenda/cashflow` | kunci internal (simulasi API eksternal) |

## Skema database

```
opd(id, kode, nama)
users(id, username, nama, password_hash, role[admin|opd|walikota], opd_id)
pekerjaan(id, opd_id, nama, nilai_kontrak, tgl_mulai, tgl_selesai, status[berjalan|selesai])
progress_pekerjaan(id, pekerjaan_id, persen, keterangan, tanggal, created_by)
bapenda_cashflow(id, tanggal, kode_rekening, uraian, jumlah, batch_id)
sync_log(id, waktu, batch_id, jumlah_baris, triggered_by)
budget_allocations(id, pekerjaan_id, cashflow_id, nominal, dibuat_oleh, dibuat_pada)
```

Rencana lengkap dan asumsi ada di [PLANNING.md](PLANNING.md).
