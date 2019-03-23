package main

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"within.website/ln"
	"within.website/ln/opname"
)

func logTemplateTime(ctx context.Context, name string, from time.Time) {
	now := time.Now()
	ln.Log(ctx, ln.F{"action": "template_rendered", "dur": now.Sub(from).String(), "name": name})
}

func (s *site) renderTemplatePage(templateFname string, data interface{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := opname.With(r.Context(), "renderTemplatePage")
		defer logTemplateTime(ctx, templateFname, time.Now())

		t, err := template.ParseFiles("templates/base.html", "templates/"+templateFname)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			ln.Error(ctx, err, ln.F{"action": "renderTemplatePage", "page": templateFname})
			return
		}

		err = t.Execute(w, data)
		if err != nil {
			panic(err)
		}
	})
}
