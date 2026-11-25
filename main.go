package main

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
)

//go:embed web
var webFS embed.FS

type App struct {
	db         *sql.DB
	pages      map[string]*template.Template
	secret     []byte
	bapendaKey string
	selfURL    string
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func rupiah(n int64) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	out = append([]string{s}, out...)
	r := "Rp " + strings.Join(out, ".")
	if neg {
		r = "-" + r
	}
	return r
}

func pct(a, b int64) int {
	if b <= 0 {
		return 0
	}
	return int(a * 100 / b)
}

func (a *App) loadTemplates() error {
	funcs := template.FuncMap{
		"rupiah": rupiah,
		"pct":    pct,
		"min100": func(n int) int { return min(n, 100) },
		"sisa":   func(a, b int64) int64 { return a - b },
	}
	a.pages = map[string]*template.Template{}
	files, err := fs.Glob(webFS, "web/templates/*.html")
	if err != nil {
		return err
	}
	for _, f := range files {
		name := strings.TrimPrefix(f, "web/templates/")
		if name == "layout.html" {
			continue
		}
		t, err := template.New(name).Funcs(funcs).ParseFS(webFS, "web/templates/layout.html", f)
		if err != nil {
			return err
		}
		a.pages[name] = t
	}
	return nil
}

func (a *App) render(w http.ResponseWriter, name string, data map[string]any) {
	t, ok := a.pages[name]
	if !ok {
		http.Error(w, "template tidak ada", 500)
		return
	}
	var buf strings.Builder
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Println("render", name, err)
		http.Error(w, "terjadi kesalahan", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

func main() {
	demo := flag.Bool("demo", false, "tambah data simulasi lalu keluar")
	flag.Parse()
	addr := env("ADDR", ":8080")
	dsn := env("DB_DSN", "root:@tcp(127.0.0.1:3306)/okebos?charset=utf8mb4&loc=Local")

	db, err := openDB(dsn)
	if err != nil {
		log.Fatal(err)
	}
	if err := seed(db); err != nil {
		log.Fatal("seed: ", err)
	}

	if *demo {
		if err := runDemo(db); err != nil {
			log.Fatal("demo: ", err)
		}
		return
	}

	secret := make([]byte, 32)
	rand.Read(secret)
	key := make([]byte, 16)
	rand.Read(key)

	a := &App{
		db:         db,
		secret:     secret,
		bapendaKey: fmt.Sprintf("%x", key),
		selfURL:    "http://127.0.0.1" + addr,
	}
	if err := a.loadTemplates(); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web/static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.loginPost)
	mux.HandleFunc("POST /logout", a.logout)

	mux.HandleFunc("GET /{$}", a.require(a.dashboard))
	mux.HandleFunc("GET /pekerjaan", a.require(a.pekerjaanList))
	mux.HandleFunc("GET /pekerjaan/new", a.require(a.pekerjaanNew, "opd", "admin"))
	mux.HandleFunc("POST /pekerjaan", a.require(a.pekerjaanCreate, "opd", "admin"))
	mux.HandleFunc("GET /pekerjaan/{id}", a.require(a.pekerjaanDetail))
	mux.HandleFunc("POST /pekerjaan/{id}/progress", a.require(a.progressCreate, "opd"))

	mux.HandleFunc("GET /admin/bapenda", a.require(a.bapendaPage, "admin"))
	mux.HandleFunc("POST /admin/bapenda/sync", a.require(a.bapendaSync, "admin"))
	mux.HandleFunc("GET /admin/alokasi", a.require(a.alokasiPage, "admin"))
	mux.HandleFunc("POST /admin/alokasi", a.require(a.alokasiCreate, "admin"))

	mux.HandleFunc("GET /admin/users", a.require(a.usersPage, "admin"))
	mux.HandleFunc("POST /admin/users", a.require(a.userCreate, "admin"))
	mux.HandleFunc("GET /admin/users/{id}/edit", a.require(a.userEditPage, "admin"))
	mux.HandleFunc("POST /admin/users/{id}", a.require(a.userUpdate, "admin"))
	mux.HandleFunc("POST /admin/users/{id}/delete", a.require(a.userDelete, "admin"))
	mux.HandleFunc("POST /admin/opd", a.require(a.opdCreate, "admin"))

	// Endpoint simulasi BAPENDA (dilindungi kunci internal, bukan sesi user).
	mux.HandleFunc("GET /api/bapenda/cashflow", a.apiCashflow)

	log.Printf("okebos berjalan di http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
