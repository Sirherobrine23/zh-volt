package page

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"runtime/debug"
	"time"

	"html/template"
)

type Page struct {
	PageBody string
	Title    string
	Data     any
}

type Pages struct {
	tmpl *template.Template
}

func (pg *Pages) Render(doc string, w io.Writer, data any) error {
	if page := pg.tmpl.Lookup(doc); page != nil {
		return page.Execute(w, data)
	}
	return fmt.Errorf("cannot process home.tmpl, not found")
}

func GetPages() (pages *Pages, err error) {
	tmpl := template.New("root")
	tmpl = tmpl.Funcs(template.FuncMap{
		"now": time.Now,
		"json_string": func(data any) string {
			if d, err := json.MarshalIndent(data, "", "  "); err == nil {
				return string(d)
			}
			return ""
		},
	})
	defer func() {
		rec := recover()
		if rec != nil {
			err = fmt.Errorf("Panic recovery: %v\n\n%s\n", rec, debug.Stack())
		}
	}()

	err = fs.WalkDir(source, ".", func(filePath string, d fs.DirEntry, err error) error {
		if err == fs.SkipAll || err == fs.SkipDir {
			return nil
		} else if err != nil {
			return err
		} else if d.IsDir() {
			return err
		}

		switch path.Ext(filePath) {
		case ".tmpl":
			if !d.Type().IsRegular() {
				return nil
			}
			f, err := source.Open(filePath)
			if err != nil {
				return err
			}
			defer f.Close()

			data, err := io.ReadAll(f)
			if err != nil {
				return err
			}

			tmpl = tmpl.New(filePath)
			_, err = tmpl.Parse(string(data))
		}

		return err
	})
	if err != nil {
		return
	}

	pages = &Pages{
		tmpl: tmpl,
	}

	return
}
