package forms824

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/saenuma/flaarum"
)

func doesPathExists(p string) bool {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return false
	}
	return true
}

func getFlaarumStmt(p string) string {
	rawJSON, err := os.ReadFile(p)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	formObjects := make([]map[string]string, 0)
	json.Unmarshal(rawJSON, &formObjects)

	tableName := strings.ReplaceAll(filepath.Base(p), ".f8p", "")
	stmt := "table: " + tableName + "\n"
	stmt += "fields:\n"
	for _, obj := range formObjects {
		var flaarumField string
		if slices.Index([]string{"email", "select", "string", "date", "datetime"}, obj["fieldtype"]) != -1 {
			flaarumField = "string"
		} else if obj["fieldtype"] == "number" {
			flaarumField = "int"
		} else if obj["fieldtype"] == "text" {
			flaarumField = "text"
		}
		attribs := strings.Split(obj["attributes"], ";")
		stmt += fmt.Sprintf("%s %s %s\n", obj["name"], flaarumField, strings.Join(attribs, " "))
	}
	stmt += "::"

	return stmt
}

type F8Object struct {
	FormsObjectPath string
	FlaarumClient   flaarum.Client
}

func Init(formObjectsPath string, cl flaarum.Client) (F8Object, error) {
	if !doesPathExists(formObjectsPath) {
		return F8Object{}, errors.New(fmt.Sprintf("formsObjectPath %s does not exists.", formObjectsPath))
	}

	if err := cl.Ping(); err != nil {
		return F8Object{}, errors.Wrap(err, "flaarum error")
	}

	dirFIs, err := os.ReadDir(formObjectsPath)
	if err != nil {
		return F8Object{}, errors.Wrap(err, "os error")
	}

	for _, dirFI := range dirFIs {
		if strings.HasSuffix(dirFI.Name(), ".f8p") {
			stmt := getFlaarumStmt(filepath.Join(formObjectsPath, dirFI.Name()))
			err = cl.CreateOrUpdateTable(stmt)
			if err != nil {
				fmt.Println(err)
			}
		}
	}

	return F8Object{formObjectsPath, cl}, nil
}

func (f8o *F8Object) getFormObjects(formName string) ([]map[string]string, error) {
	if !strings.HasSuffix(formName, ".f8p") {
		formName += ".f8p"
	}
	formPath := filepath.Join(f8o.FormsObjectPath, formName)

	rawJSON, err := os.ReadFile(formPath)
	if err != nil {
		return nil, errors.Wrap(err, "json error")
	}

	formObjects := make([]map[string]string, 0)
	json.Unmarshal(rawJSON, &formObjects)

	return formObjects, nil
}

func (f8o *F8Object) GetNewForm(formName string) (string, error) {

	formObjects, err := f8o.getFormObjects(formName)
	if err != nil {
		return "", err
	}

	var html string
	for _, obj := range formObjects {
		if slices.Index(strings.Split(obj["attributes"], ";"), "hidden") != -1 {
			continue
		}

		html += "<div>"
		html += fmt.Sprintf("<div><label for='id_%s'>%s</label></div>", obj["name"], obj["label"])
		if slices.Index([]string{"number", "string", "email", "date", "datetime"}, obj["fieldtype"]) != -1 {
			html += fmt.Sprintf("<input type='%s' name='%s' id='id_%s' ", obj["fieldtype"],
				obj["name"], obj["name"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += "/>"
		} else if obj["fieldtype"] == "select" {
			html += fmt.Sprintf("<select id='id_%s' name='%s'", obj["name"], obj["name"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += ">"
			for _, opt := range strings.Split(obj["select_options"], "\n") {
				html += "<option>" + opt + "</option>"
			}
			html += "</select>"
		} else if obj["fieldtype"] == "text" {
			html += fmt.Sprintf("<textarea id='id_%s' name='%s'", obj["name"], obj["name"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += "><textarea/>"
		}
		html += "</div>"
	}

	return html, nil
}


func (f8o *F8Object) GetEditForm(formName string, oldData map[string]string) (string, error) {
	formObjects, err := f8o.getFormObjects(formName)
	if err != nil {
		return "", err
	}

	var html string
	for _, obj := range formObjects {
		if slices.Index(strings.Split(obj["attributes"], ";"), "hidden") != -1 {
			continue
		}
		var currentOldData string
		tmpValue, ok := oldData[obj["name"]]
		if ok {
			currentOldData = tmpValue
		}
		
		html += "<div>"
		html += fmt.Sprintf("<div><label for='id_%s'>%s</label></div>", obj["name"], obj["label"])
		if slices.Index([]string{"number", "string", "email", "date", "datetime"}, obj["fieldtype"]) != -1 {
			html += fmt.Sprintf("<input type='%s' name='%s' id='id_%s' value='%s'", obj["fieldtype"],
				obj["name"], obj["name"], currentOldData)
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += "/>"
		} else if obj["fieldtype"] == "select" {
			html += fmt.Sprintf("<select id='id_%s' name='%s'", obj["name"], obj["name"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += ">"
			for _, opt := range strings.Split(obj["select_options"], "\n") {
				if opt == currentOldData {
					html += "<option selected='true'>" + opt + "</option>"
				} else {
					html += "<option>" + opt + "</option>"					
				}
			}
			html += "</select>"
		} else if obj["fieldtype"] == "text" {
			html += fmt.Sprintf("<textarea id='id_%s' name='%s'", obj["name"], obj["name"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += ">" + currentOldData + "<textarea/>"
		}
		html += "</div>"
	}

	return html, nil
}
