package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"

	"github.com/saenuma/flaarum"
	"github.com/saenuma/forms824"
)

func main() {
	cl := flaarum.NewClient("127.0.0.1", "not-yet-ready", "f8proj")
	f8cl, err := forms824.Init("forms", cl)
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			fmt.Fprintf(w, "not found")
			return
		}

		forms, err := f8cl.ListForms()
		if err != nil {
			fmt.Fprintf(w, "error with forms")
			return
		}

		type Ctx struct {
			Forms []string
		}

		tmpl := template.Must(template.ParseFiles("templates/home.html"))
		tmpl.Execute(w, Ctx{forms})
	})

	http.HandleFunc("/insert/{form}", func(w http.ResponseWriter, r *http.Request) {
		formName := r.PathValue("form")

		if r.Method == http.MethodGet {
			html, err := f8cl.GetNewForm(formName)
			if err != nil {
				panic(err)
			}
			type RCtx struct {
				FormName string
				FormHTML template.HTML
			}
			tmpl := template.Must(template.ParseFiles("templates/insert.html"))
			tmpl.Execute(w, RCtx{formName, template.HTML(html)})

		} else {
			toWrite, err := f8cl.GetSubmittedData(r, formName)
			if err != nil {
				panic(err)
			}
			retId, err := cl.InsertRowStr(formName, toWrite)
			if err != nil {
				panic(err)
			}

			fmt.Fprintf(w, "done. id #%d", retId)
		}

	})

	http.HandleFunc("/edit/{form}/{id}", func(w http.ResponseWriter, r *http.Request) {
		formName := r.PathValue("form")
		dataIdStr := r.PathValue("id")
		dataId, err := strconv.ParseInt(dataIdStr, 10, 64)
		if err != nil {
			panic(err)
		}

		if r.Method == http.MethodGet {
			html, err := f8cl.GetEditForm(formName, dataId)
			if err != nil {
				panic(err)
			}
			type RCtx struct {
				FormName string
				DataId   int64
				FormHTML template.HTML
			}
			tmpl := template.Must(template.ParseFiles("templates/edit.html"))
			tmpl.Execute(w, RCtx{formName, dataId, template.HTML(html)})
		} else {
			toWrite, err := f8cl.GetSubmittedData(r, formName)
			if err != nil {
				panic(err)
			}
			err = cl.UpdateRowsStr(fmt.Sprintf(`
				table: %s
				where:
					id = %d
				`, formName, dataId), toWrite)
			if err != nil {
				panic(err)
			}

			fmt.Fprintf(w, "updated.")
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}
