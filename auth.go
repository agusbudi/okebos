package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID       int
	Username string
	Nama     string
	Role     string
	OPDID    int // 0 bila bukan OPD
}

type ctxKey struct{}

const cookieName = "okebos_session"

func (a *App) sign(s string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (a *App) setSession(w http.ResponseWriter, uid int) {
	val := fmt.Sprintf("%d|%d", uid, time.Now().Add(8*time.Hour).Unix())
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    base64.RawURLEncoding.EncodeToString([]byte(val)) + "." + a.sign(val),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   8 * 3600,
	})
}

func (a *App) currentUser(r *http.Request) *User {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	enc, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil || !hmac.Equal([]byte(sig), []byte(a.sign(string(raw)))) {
		return nil
	}
	idStr, expStr, _ := strings.Cut(string(raw), "|")
	exp, _ := strconv.ParseInt(expStr, 10, 64)
	id, _ := strconv.Atoi(idStr)
	if time.Now().Unix() > exp || id == 0 {
		return nil
	}
	u := &User{}
	var opd *int
	err = a.db.QueryRow("SELECT id,username,nama,role,opd_id FROM users WHERE id=?", id).
		Scan(&u.ID, &u.Username, &u.Nama, &u.Role, &opd)
	if err != nil {
		return nil
	}
	if opd != nil {
		u.OPDID = *opd
	}
	return u
}

// require mewajibkan login dan (opsional) membatasi peran.
func (a *App) require(h func(http.ResponseWriter, *http.Request, *User), roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := a.currentUser(r)
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if len(roles) > 0 {
			ok := false
			for _, ro := range roles {
				ok = ok || ro == u.Role
			}
			if !ok {
				http.Error(w, "Akses ditolak", http.StatusForbidden)
				return
			}
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)), u)
	}
}

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	if a.currentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.render(w, "login.html", map[string]any{"Err": r.URL.Query().Get("err")})
}

func (a *App) loginPost(w http.ResponseWriter, r *http.Request) {
	username, pw := r.FormValue("username"), r.FormValue("password")
	var id int
	var h string
	err := a.db.QueryRow("SELECT id,password_hash FROM users WHERE username=?", username).Scan(&id, &h)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(h), []byte(pw)) != nil {
		http.Redirect(w, r, "/login?err=Username+atau+password+salah", http.StatusSeeOther)
		return
	}
	a.setSession(w, id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
