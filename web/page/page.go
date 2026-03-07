package page

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/olt"
)

func NewPage(maneger *olt.OltManeger) *http.ServeMux {
	pageRoute := http.NewServeMux()
	static, err := fs.Sub(source, "static")
	if err != nil {
		panic(err)
	}
	pageRoute.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	pageRoute.HandleFunc("GET /_update/{onuMAC}", func(w http.ResponseWriter, r *http.Request) {
		pg, err := GetPages()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error on render page: %s\n", err)
			w.Header().Set("Content-Type", "text/plain; utf-8")
			w.WriteHeader(500)
			fmt.Fprintf(w, "error on render page: %s\n\n", err)
			return
		}

		olt := maneger.Olts()[r.PathValue("onuMAC")]
		if olt == nil {
			w.Header().Set("Content-Type", "text/plain; utf-8")
			w.WriteHeader(404)
			fmt.Fprintln(w, "OLT not found")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, private")
		err = pg.Render("olt.tmpl", w, olt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error on render page: %s\n", err)
			w.Header().Set("Content-Type", "text/plain; utf-8")
			w.WriteHeader(500)
			fmt.Fprintf(w, "error on render page: %s\n\n", err)
		}
	})

	pageRoute.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, private")

		pg, err := GetPages()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error on render page: %s\n", err)
			w.Header().Set("Content-Type", "text/plain; utf-8")
			w.WriteHeader(500)
			fmt.Fprintf(w, "error on render page: %s\n\n", err)
			return
		}

		err = pg.Render("main.tmpl", w, maneger.Olts())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error on render page: %s\n", err)
			w.Header().Set("Content-Type", "text/plain; utf-8")
			w.WriteHeader(500)
			fmt.Fprintf(w, "error on render page: %s\n\n", err)
		}
	})

	return pageRoute
}
