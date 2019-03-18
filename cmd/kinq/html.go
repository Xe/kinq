package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"time"

	"within.website/ln"
)

func logTemplateTime(name string, from time.Time) {
	now := time.Now()
	ln.Log(context.Background(), ln.F{"action": "template_rendered", "dur": now.Sub(from).String(), "name": name})
}

func (s *site) renderTemplatePage(templateFname string, data interface{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer logTemplateTime(templateFname, time.Now())
		s.tlock.RLock()
		defer s.tlock.RUnlock()

		var t *template.Template
		var err error

		if s.templates[templateFname] == nil {
			t, err = template.ParseFiles("templates/base.html", "templates/"+templateFname)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				ln.Error(context.Background(), err, ln.F{"action": "renderTemplatePage", "page": templateFname})
				return
			}

			if t == nil && err == nil {
				log.Fatal("???")
			}

			ln.Log(context.Background(), ln.F{"action": "loaded_new_template", "fname": templateFname})

			s.tlock.RUnlock()
			s.tlock.Lock()
			s.templates[templateFname] = t
			s.tlock.Unlock()
			s.tlock.RLock()
		} else {
			t = s.templates[templateFname]
		}

		if t == nil {
			panic("this shouldn't happen")
		}

		err = t.Execute(w, data)
		if err != nil {
			panic(err)
		}
	})
}
