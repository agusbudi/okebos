package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Pekerjaan struct {
	ID, OPDID      int
	OPD, Nama      string
	Nilai, Alokasi int64
	Mulai, Selesai string
	Status         string
	Persen         int
}

const pekerjaanSelect = `
SELECT p.id,p.opd_id,o.nama,p.nama,p.nilai_kontrak,p.tgl_mulai,p.tgl_selesai,p.status,
  COALESCE((SELECT pr.persen FROM progress_pekerjaan pr WHERE pr.pekerjaan_id=p.id ORDER BY pr.id DESC LIMIT 1),0),
  COALESCE((SELECT SUM(b.nominal) FROM budget_allocations b WHERE b.pekerjaan_id=p.id),0)
FROM pekerjaan p JOIN opd o ON o.id=p.opd_id `

func (a *App) queryPekerjaan(where string, args ...any) ([]Pekerjaan, error) {
	rows, err := a.db.Query(pekerjaanSelect+where+" ORDER BY o.nama,p.id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pekerjaan
	for rows.Next() {
		var p Pekerjaan
		if err := rows.Scan(&p.ID, &p.OPDID, &p.OPD, &p.Nama, &p.Nilai, &p.Mulai, &p.Selesai, &p.Status, &p.Persen, &p.Alokasi); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// scope mengembalikan klausa WHERE yang membatasi OPD ke datanya sendiri.
// Pembatasan dilakukan di query, bukan hanya di tampilan.
func scope(u *User, extra string) (string, []any) {
	if u.Role == "opd" {
		return "WHERE p.opd_id=? " + extra, []any{u.OPDID}
	}
	return "WHERE 1=1 " + extra, nil
}

func (a *App) serverErr(w http.ResponseWriter, err error) {
	log.Println("error:", err)
	http.Error(w, "Terjadi kesalahan server", http.StatusInternalServerError)
}

func redirectMsg(w http.ResponseWriter, r *http.Request, path, key, msg string) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	http.Redirect(w, r, path+sep+key+"="+url.QueryEscape(msg), http.StatusSeeOther)
}

func base(u *User, r *http.Request) map[string]any {
	return map[string]any{
		"User": u,
		"Msg":  r.URL.Query().Get("msg"),
		"Err":  r.URL.Query().Get("err"),
	}
}

// ---------- Dashboard ----------

type ChartData struct {
	Labels   []string `json:"labels"`
	Progress []int    `json:"progress"`
	Serapan  []int    `json:"serapan"`
}

type OPDAgg struct {
	Nama            string
	Jumlah, Selesai int
	SumPersen       int
	Nilai, Alokasi  int64
}

func (g *OPDAgg) Avg() int     { return g.SumPersen / g.Jumlah }
func (g *OPDAgg) Serapan() int { return pct(g.Alokasi, g.Nilai) }

func (a *App) dashboard(w http.ResponseWriter, r *http.Request, u *User) {
	where, args := scope(u, "")
	list, err := a.queryPekerjaan(where, args...)
	if err != nil {
		a.serverErr(w, err)
		return
	}

	var selesai, sumPersen int
	var nilai, alokasi int64
	var order []int
	agg := map[int]*OPDAgg{}
	for _, p := range list {
		if p.Status == "selesai" {
			selesai++
		}
		sumPersen += p.Persen
		nilai += p.Nilai
		alokasi += p.Alokasi
		g, ok := agg[p.OPDID]
		if !ok {
			g = &OPDAgg{Nama: p.OPD}
			agg[p.OPDID] = g
			order = append(order, p.OPDID)
		}
		g.Jumlah++
		g.SumPersen += p.Persen
		g.Nilai += p.Nilai
		g.Alokasi += p.Alokasi
		if p.Status == "selesai" {
			g.Selesai++
		}
	}
	avg := 0
	if len(list) > 0 {
		avg = sumPersen / len(list)
	}

	var chart ChartData
	var perOPD []*OPDAgg
	for _, id := range order {
		perOPD = append(perOPD, agg[id])
	}
	if u.Role == "opd" {
		for _, p := range list {
			chart.Labels = append(chart.Labels, p.Nama)
			chart.Progress = append(chart.Progress, p.Persen)
			chart.Serapan = append(chart.Serapan, pct(p.Alokasi, p.Nilai))
		}
	} else {
		for _, g := range perOPD {
			chart.Labels = append(chart.Labels, g.Nama)
			chart.Progress = append(chart.Progress, g.Avg())
			chart.Serapan = append(chart.Serapan, g.Serapan())
		}
	}

	d := base(u, r)
	d["Total"], d["Selesai"], d["Berjalan"], d["Avg"] = len(list), selesai, len(list)-selesai, avg
	d["Nilai"], d["Alokasi"], d["Serapan"] = nilai, alokasi, pct(alokasi, nilai)
	d["Chart"] = chart
	d["Pekerjaan"] = list
	if u.Role != "opd" {
		var cash int64
		a.db.QueryRow("SELECT COALESCE(SUM(jumlah),0) FROM bapenda_cashflow").Scan(&cash)
		d["Cashflow"], d["Saldo"] = cash, cash-alokasi
		d["PerOPD"] = perOPD
	}
	a.render(w, "dashboard.html", d)
}

// ---------- Pekerjaan ----------

func (a *App) pekerjaanList(w http.ResponseWriter, r *http.Request, u *User) {
	extra, filterOPD, status := "", 0, r.URL.Query().Get("status")
	var args []any
	if u.Role != "opd" {
		if id, _ := strconv.Atoi(r.URL.Query().Get("opd")); id > 0 {
			filterOPD = id
			extra += "AND p.opd_id=? "
			args = append(args, id)
		}
	}
	if status == "berjalan" || status == "selesai" {
		extra += "AND p.status=? "
		args = append(args, status)
	} else {
		status = ""
	}
	where, sargs := scope(u, extra)
	list, err := a.queryPekerjaan(where, append(sargs, args...)...)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	d := base(u, r)
	d["Pekerjaan"], d["FilterOPD"], d["FilterStatus"] = list, filterOPD, status
	if u.Role != "opd" {
		d["OPDs"] = a.listOPD()
	}
	a.render(w, "pekerjaan_list.html", d)
}

type OPDItem struct {
	ID         int
	Kode, Nama string
}

func (a *App) listOPD() []OPDItem {
	rows, err := a.db.Query("SELECT id,kode,nama FROM opd ORDER BY nama")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []OPDItem
	for rows.Next() {
		var o OPDItem
		rows.Scan(&o.ID, &o.Kode, &o.Nama)
		out = append(out, o)
	}
	return out
}

type ProgressRow struct {
	Persen              int
	Keterangan, Tanggal string
	Oleh                string
}

type AlokasiRow struct {
	ID                int
	Pekerjaan, OPD    string
	Uraian, Kode, Tgl string
	Nominal           int64
}

func (a *App) pekerjaanDetail(w http.ResponseWriter, r *http.Request, u *User) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	where, args := scope(u, "AND p.id=?")
	list, err := a.queryPekerjaan(where, append(args, id)...)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	if len(list) == 0 { // bukan milik OPD ini / tidak ada -> 404 agar tidak membocorkan keberadaan
		http.NotFound(w, r)
		return
	}
	p := list[0]

	var progress []ProgressRow
	rows, err := a.db.Query(`SELECT pr.persen,pr.keterangan,pr.tanggal,u.nama FROM progress_pekerjaan pr
		JOIN users u ON u.id=pr.created_by WHERE pr.pekerjaan_id=? ORDER BY pr.id DESC`, p.ID)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	for rows.Next() {
		var x ProgressRow
		rows.Scan(&x.Persen, &x.Keterangan, &x.Tanggal, &x.Oleh)
		progress = append(progress, x)
	}
	rows.Close()

	var alok []AlokasiRow
	rows, err = a.db.Query(`SELECT b.id,c.uraian,c.kode_rekening,c.tanggal,b.nominal FROM budget_allocations b
		JOIN bapenda_cashflow c ON c.id=b.cashflow_id WHERE b.pekerjaan_id=? ORDER BY b.id DESC`, p.ID)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	for rows.Next() {
		var x AlokasiRow
		rows.Scan(&x.ID, &x.Uraian, &x.Kode, &x.Tgl, &x.Nominal)
		alok = append(alok, x)
	}
	rows.Close()

	d := base(u, r)
	d["P"], d["Progress"], d["Alokasi"] = p, progress, alok
	d["Today"] = time.Now().Format("2006-01-02")
	d["Serapan"] = pct(p.Alokasi, p.Nilai)
	a.render(w, "pekerjaan_detail.html", d)
}

func (a *App) pekerjaanNew(w http.ResponseWriter, r *http.Request, u *User) {
	d := base(u, r)
	if u.Role == "admin" {
		d["OPDs"] = a.listOPD()
	}
	a.render(w, "pekerjaan_new.html", d)
}

func (a *App) pekerjaanCreate(w http.ResponseWriter, r *http.Request, u *User) {
	opdID := u.OPDID // OPD selalu memakai OPD miliknya, input form diabaikan
	if u.Role == "admin" {
		opdID, _ = strconv.Atoi(r.FormValue("opd_id"))
	}
	nama := strings.TrimSpace(r.FormValue("nama"))
	nilai, _ := parseRupiah(r.FormValue("nilai"))
	mulai, selesai := r.FormValue("tgl_mulai"), r.FormValue("tgl_selesai")
	_, e1 := time.Parse("2006-01-02", mulai)
	_, e2 := time.Parse("2006-01-02", selesai)
	if opdID == 0 || nama == "" || len(nama) > 200 || nilai <= 0 || e1 != nil || e2 != nil || selesai < mulai {
		redirectMsg(w, r, "/pekerjaan/new", "err", "Data tidak valid. Periksa semua isian.")
		return
	}
	res, err := a.db.Exec("INSERT INTO pekerjaan(opd_id,nama,nilai_kontrak,tgl_mulai,tgl_selesai) VALUES(?,?,?,?,?)",
		opdID, nama, nilai, mulai, selesai)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	id, _ := res.LastInsertId()
	redirectMsg(w, r, fmt.Sprintf("/pekerjaan/%d", id), "msg", "Pekerjaan ditambahkan")
}

func (a *App) progressCreate(w http.ResponseWriter, r *http.Request, u *User) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	back := fmt.Sprintf("/pekerjaan/%d", id)
	where, args := scope(u, "AND p.id=?")
	list, err := a.queryPekerjaan(where, append(args, id)...)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	if len(list) == 0 {
		http.NotFound(w, r)
		return
	}
	p := list[0]
	persen, perr := strconv.Atoi(r.FormValue("persen"))
	tgl := r.FormValue("tanggal")
	_, terr := time.Parse("2006-01-02", tgl)
	switch {
	case p.Status == "selesai":
		redirectMsg(w, r, back, "err", "Pekerjaan sudah selesai")
	case perr != nil || persen < 0 || persen > 100 || terr != nil:
		redirectMsg(w, r, back, "err", "Persen (0-100) atau tanggal tidak valid")
	case persen < p.Persen:
		redirectMsg(w, r, back, "err", fmt.Sprintf("Progres tidak boleh turun dari %d%%", p.Persen))
	default:
		tx, err := a.db.Begin()
		if err != nil {
			a.serverErr(w, err)
			return
		}
		defer tx.Rollback()
		ket := r.FormValue("keterangan")
		if len(ket) > 255 {
			ket = ket[:255]
		}
		_, err = tx.Exec("INSERT INTO progress_pekerjaan(pekerjaan_id,persen,keterangan,tanggal,created_by) VALUES(?,?,?,?,?)",
			p.ID, persen, ket, tgl, u.ID)
		if err == nil && persen == 100 {
			_, err = tx.Exec("UPDATE pekerjaan SET status='selesai' WHERE id=?", p.ID)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			a.serverErr(w, err)
			return
		}
		redirectMsg(w, r, back, "msg", "Progres disimpan")
	}
}

// ---------- Admin: BAPENDA & alokasi ----------

type CashRow struct {
	ID                    int
	Tanggal, Kode, Uraian string
	Batch                 string
	Jumlah, Saldo         int64
}

type SyncRow struct {
	Waktu, Batch, Oleh string
	Baris              int
}

func (a *App) queryCash(having string) ([]CashRow, error) {
	rows, err := a.db.Query(`SELECT c.id,c.tanggal,c.kode_rekening,c.uraian,c.batch_id,c.jumlah,
		c.jumlah-COALESCE((SELECT SUM(nominal) FROM budget_allocations WHERE cashflow_id=c.id),0) AS saldo
		FROM bapenda_cashflow c ` + having + ` ORDER BY c.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CashRow
	for rows.Next() {
		var c CashRow
		if err := rows.Scan(&c.ID, &c.Tanggal, &c.Kode, &c.Uraian, &c.Batch, &c.Jumlah, &c.Saldo); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (a *App) bapendaPage(w http.ResponseWriter, r *http.Request, u *User) {
	d := base(u, r)
	cash, err := a.queryCash("")
	if err != nil {
		a.serverErr(w, err)
		return
	}
	if len(cash) > 50 {
		cash = cash[:50]
	}
	rows, err := a.db.Query(`SELECT s.waktu,s.batch_id,s.jumlah_baris,u.nama FROM sync_log s
		JOIN users u ON u.id=s.triggered_by ORDER BY s.id DESC LIMIT 20`)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	defer rows.Close()
	var logs []SyncRow
	for rows.Next() {
		var s SyncRow
		rows.Scan(&s.Waktu, &s.Batch, &s.Baris, &s.Oleh)
		logs = append(logs, s)
	}
	d["Cash"], d["Logs"] = cash, logs
	a.render(w, "bapenda.html", d)
}

func (a *App) bapendaSync(w http.ResponseWriter, r *http.Request, u *User) {
	b, err := a.pullBapenda(u)
	if err != nil {
		log.Println("sync bapenda:", err)
		redirectMsg(w, r, "/admin/bapenda", "err", "Gagal menarik data BAPENDA: "+err.Error())
		return
	}
	redirectMsg(w, r, "/admin/bapenda", "msg", fmt.Sprintf("Berhasil menarik %d transaksi (batch %s)", len(b.Data), b.BatchID))
}

func (a *App) alokasiPage(w http.ResponseWriter, r *http.Request, u *User) {
	cash, err := a.queryCash("HAVING saldo > 0")
	if err != nil {
		a.serverErr(w, err)
		return
	}
	works, err := a.queryPekerjaan("WHERE p.status='berjalan'")
	if err != nil {
		a.serverErr(w, err)
		return
	}
	rows, err := a.db.Query(`SELECT b.id,p.nama,o.nama,c.uraian,c.kode_rekening,b.dibuat_pada,b.nominal
		FROM budget_allocations b JOIN pekerjaan p ON p.id=b.pekerjaan_id JOIN opd o ON o.id=p.opd_id
		JOIN bapenda_cashflow c ON c.id=b.cashflow_id ORDER BY b.id DESC LIMIT 50`)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	defer rows.Close()
	var alok []AlokasiRow
	for rows.Next() {
		var x AlokasiRow
		rows.Scan(&x.ID, &x.Pekerjaan, &x.OPD, &x.Uraian, &x.Kode, &x.Tgl, &x.Nominal)
		alok = append(alok, x)
	}
	d := base(u, r)
	d["Cash"], d["Works"], d["Alokasi"] = cash, works, alok
	a.render(w, "alokasi.html", d)
}

func (a *App) alokasiCreate(w http.ResponseWriter, r *http.Request, u *User) {
	pid, _ := strconv.Atoi(r.FormValue("pekerjaan_id"))
	cid, _ := strconv.Atoi(r.FormValue("cashflow_id"))
	nominal, _ := parseRupiah(r.FormValue("nominal"))
	fail := func(m string) { redirectMsg(w, r, "/admin/alokasi", "err", m) }
	if pid == 0 || cid == 0 || nominal <= 0 {
		fail("Pilih pekerjaan, sumber cashflow, dan nominal > 0")
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		a.serverErr(w, err)
		return
	}
	defer tx.Rollback()

	// Kunci baris cashflow dan pekerjaan supaya alokasi bersamaan tidak melebihi batas.
	var jumlah int64
	if err := tx.QueryRow("SELECT jumlah FROM bapenda_cashflow WHERE id=? FOR UPDATE", cid).Scan(&jumlah); err != nil {
		if err == sql.ErrNoRows {
			fail("Sumber cashflow tidak ditemukan")
			return
		}
		a.serverErr(w, err)
		return
	}
	var terpakai int64
	tx.QueryRow("SELECT COALESCE(SUM(nominal),0) FROM budget_allocations WHERE cashflow_id=?", cid).Scan(&terpakai)
	if nominal > jumlah-terpakai {
		fail("Nominal melebihi saldo cashflow (" + rupiah(jumlah-terpakai) + ")")
		return
	}

	var nilai, sudah int64
	if err := tx.QueryRow("SELECT nilai_kontrak FROM pekerjaan WHERE id=? FOR UPDATE", pid).Scan(&nilai); err != nil {
		fail("Pekerjaan tidak ditemukan")
		return
	}
	tx.QueryRow("SELECT COALESCE(SUM(nominal),0) FROM budget_allocations WHERE pekerjaan_id=?", pid).Scan(&sudah)
	if sudah+nominal > nilai {
		fail("Total alokasi melebihi nilai kontrak pekerjaan (sisa " + rupiah(nilai-sudah) + ")")
		return
	}

	if _, err := tx.Exec("INSERT INTO budget_allocations(pekerjaan_id,cashflow_id,nominal,dibuat_oleh,dibuat_pada) VALUES(?,?,?,?,NOW())",
		pid, cid, nominal, u.ID); err != nil {
		a.serverErr(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		a.serverErr(w, err)
		return
	}
	redirectMsg(w, r, "/admin/alokasi", "msg", "Anggaran dialokasikan ke pekerjaan")
}

// parseRupiah menerima angka berformat ("1.500.000") maupun polos; hanya digit yang dipakai.
func parseRupiah(s string) (int64, error) {
	var b strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return strconv.ParseInt(b.String(), 10, 64)
}
