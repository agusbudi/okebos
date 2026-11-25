package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
)

var (
	usernameRe = regexp.MustCompile(`^[a-z0-9_]{3,50}$`)
	kodeRe     = regexp.MustCompile(`^[A-Za-z0-9_-]{2,20}$`)
)

const minPassword = 6

type UserRow struct {
	ID                        int
	Username, Nama, Role, OPD string
	OPDID                     int
}

func isDup(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func (a *App) usersPage(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := a.db.Query(`SELECT u.id,u.username,u.nama,u.role,COALESCE(u.opd_id,0),COALESCE(o.nama,'')
		FROM users u LEFT JOIN opd o ON o.id=u.opd_id ORDER BY FIELD(u.role,'admin','walikota','opd'),o.nama,u.username`)
	if err != nil {
		a.serverErr(w, err)
		return
	}
	defer rows.Close()
	var users []UserRow
	for rows.Next() {
		var x UserRow
		rows.Scan(&x.ID, &x.Username, &x.Nama, &x.Role, &x.OPDID, &x.OPD)
		users = append(users, x)
	}
	d := base(u, r)
	d["Users"], d["OPDs"] = users, a.listOPD()
	a.render(w, "users.html", d)
}

// validateUserForm memeriksa isian user; mengembalikan opd_id (nil bila bukan OPD).
func (a *App) validateUserForm(r *http.Request) (nama, role string, opd any, msg string) {
	nama = strings.TrimSpace(r.FormValue("nama"))
	role = r.FormValue("role")
	if nama == "" || len(nama) > 100 {
		return "", "", nil, "Nama wajib diisi (maks. 100 karakter)"
	}
	switch role {
	case "admin", "walikota":
		return nama, role, nil, ""
	case "opd":
		id, _ := strconv.Atoi(r.FormValue("opd_id"))
		var n int
		a.db.QueryRow("SELECT COUNT(*) FROM opd WHERE id=?", id).Scan(&n)
		if n == 0 {
			return "", "", nil, "Pilih OPD untuk user berperan OPD"
		}
		return nama, role, id, ""
	}
	return "", "", nil, "Peran tidak valid"
}

func (a *App) userCreate(w http.ResponseWriter, r *http.Request, u *User) {
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	pw := r.FormValue("password")
	nama, role, opd, msg := a.validateUserForm(r)
	switch {
	case msg != "":
	case !usernameRe.MatchString(username):
		msg = "Username 3-50 karakter: huruf kecil, angka, atau underscore"
	case len(pw) < minPassword:
		msg = fmt.Sprintf("Password minimal %d karakter", minPassword)
	}
	if msg != "" {
		redirectMsg(w, r, "/admin/users", "err", msg)
		return
	}
	_, err := a.db.Exec("INSERT INTO users(username,nama,password_hash,role,opd_id) VALUES(?,?,?,?,?)",
		username, nama, hash(pw), role, opd)
	if isDup(err) {
		redirectMsg(w, r, "/admin/users", "err", "Username sudah dipakai")
		return
	}
	if err != nil {
		a.serverErr(w, err)
		return
	}
	redirectMsg(w, r, "/admin/users", "msg", "User "+username+" ditambahkan")
}

func (a *App) userEditPage(w http.ResponseWriter, r *http.Request, u *User) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var x UserRow
	err := a.db.QueryRow("SELECT id,username,nama,role,COALESCE(opd_id,0) FROM users WHERE id=?", id).
		Scan(&x.ID, &x.Username, &x.Nama, &x.Role, &x.OPDID)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.serverErr(w, err)
		return
	}
	d := base(u, r)
	d["U"], d["OPDs"], d["Self"] = x, a.listOPD(), x.ID == u.ID
	a.render(w, "user_edit.html", d)
}

func (a *App) adminCount(excludeID int) int {
	var n int
	a.db.QueryRow("SELECT COUNT(*) FROM users WHERE role='admin' AND id<>?", excludeID).Scan(&n)
	return n
}

func (a *App) userUpdate(w http.ResponseWriter, r *http.Request, u *User) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	back := fmt.Sprintf("/admin/users/%d/edit", id)
	var oldRole string
	if err := a.db.QueryRow("SELECT role FROM users WHERE id=?", id).Scan(&oldRole); err != nil {
		http.NotFound(w, r)
		return
	}
	nama, role, opd, msg := a.validateUserForm(r)
	pw := r.FormValue("password")
	switch {
	case msg != "":
	case pw != "" && len(pw) < minPassword:
		msg = fmt.Sprintf("Password minimal %d karakter", minPassword)
	case oldRole == "admin" && role != "admin" && a.adminCount(id) == 0:
		msg = "Tidak bisa mengubah peran: harus ada minimal satu admin"
	case id == u.ID && role != oldRole:
		msg = "Anda tidak bisa mengubah peran akun sendiri"
	}
	if msg != "" {
		redirectMsg(w, r, back, "err", msg)
		return
	}
	var err error
	if pw != "" {
		_, err = a.db.Exec("UPDATE users SET nama=?,role=?,opd_id=?,password_hash=? WHERE id=?", nama, role, opd, hash(pw), id)
	} else {
		_, err = a.db.Exec("UPDATE users SET nama=?,role=?,opd_id=? WHERE id=?", nama, role, opd, id)
	}
	if err != nil {
		a.serverErr(w, err)
		return
	}
	redirectMsg(w, r, "/admin/users", "msg", "User diperbarui")
}

func (a *App) userDelete(w http.ResponseWriter, r *http.Request, u *User) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var role string
	if err := a.db.QueryRow("SELECT role FROM users WHERE id=?", id).Scan(&role); err != nil {
		http.NotFound(w, r)
		return
	}
	var used int
	a.db.QueryRow(`SELECT (SELECT COUNT(*) FROM progress_pekerjaan WHERE created_by=?)
		+(SELECT COUNT(*) FROM sync_log WHERE triggered_by=?)
		+(SELECT COUNT(*) FROM budget_allocations WHERE dibuat_oleh=?)`, id, id, id).Scan(&used)
	switch {
	case id == u.ID:
		redirectMsg(w, r, "/admin/users", "err", "Anda tidak bisa menghapus akun sendiri")
	case role == "admin" && a.adminCount(id) == 0:
		redirectMsg(w, r, "/admin/users", "err", "Harus ada minimal satu admin")
	case used > 0:
		redirectMsg(w, r, "/admin/users", "err", "User sudah punya riwayat laporan/aktivitas sehingga tidak bisa dihapus. Ubah password atau peran saja.")
	default:
		if _, err := a.db.Exec("DELETE FROM users WHERE id=?", id); err != nil {
			a.serverErr(w, err)
			return
		}
		redirectMsg(w, r, "/admin/users", "msg", "User dihapus")
	}
}

func (a *App) opdCreate(w http.ResponseWriter, r *http.Request, u *User) {
	kode := strings.ToUpper(strings.TrimSpace(r.FormValue("kode")))
	nama := strings.TrimSpace(r.FormValue("nama"))
	if !kodeRe.MatchString(kode) || nama == "" || len(nama) > 120 {
		redirectMsg(w, r, "/admin/users", "err", "Kode OPD 2-20 karakter (huruf/angka) dan nama wajib diisi")
		return
	}
	_, err := a.db.Exec("INSERT INTO opd(kode,nama) VALUES(?,?)", kode, nama)
	if isDup(err) {
		redirectMsg(w, r, "/admin/users", "err", "Kode OPD sudah ada")
		return
	}
	if err != nil {
		a.serverErr(w, err)
		return
	}
	redirectMsg(w, r, "/admin/users", "msg", "OPD "+nama+" ditambahkan")
}
