package forms824

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"net/http"

	"github.com/pkg/errors"
	"github.com/saenuma/flaarum"
)

func doesPathExists(p string) bool {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return false
	}
	return true
}

func getFormObjects(formObjectsPath, formName string) ([]map[string]string, error) {
	if !strings.HasSuffix(formName, ".f8p") {
		formName += ".f8p"
	}
	formPath := filepath.Join(formObjectsPath, formName)

	rawJSON, err := os.ReadFile(formPath)
	if err != nil {
		return nil, errors.Wrap(err, "json error")
	}

	formObjects := make([]map[string]string, 0)
	json.Unmarshal(rawJSON, &formObjects)

	return formObjects, nil
}

func getFlaarumStmt(formObjectsPath, formName string) string {
	formObjects, err := getFormObjects(formObjectsPath, formName)
	if err != nil {
		return ""
	}

	tableName := strings.ReplaceAll(formName, ".f8p", "")
	stmt := "table: " + tableName + "\n"
	stmt += "fields:\n"

	hasForeignKeys := false
	var stmtSuffix string
	for _, obj := range formObjects {
		var flaarumField string
		stringLikeFields := []string{"email", "select", "string", "date", "datetime", 
			"multi_display_select", "single_display_select", "check"}
		if slices.Index(stringLikeFields, obj["fieldtype"]) != -1 {
			flaarumField = "string"
		} else if obj["fieldtype"] == "number" {
			flaarumField = "int"

			if val, ok := obj["linked_table"]; ok{
				hasForeignKeys = true
				stmtSuffix += fmt.Sprintf("%s %s on_delete_delete \n", obj["name"], val)
			}

		} else if obj["fieldtype"] == "text" {
			flaarumField = "text"
		}
		attribs := strings.Split(obj["attributes"], ";")
		stmt += fmt.Sprintf("%s %s %s\n", obj["name"], flaarumField, strings.Join(attribs, " "))
	}
	stmt += "::"

	if hasForeignKeys {
		stmt += "foreign_keys:\n" + stmt + "\n::"
	}

	return stmt
}


func getForeignKey(formObjectsPath, formName string) string {
	formObjects, err := getFormObjects(formObjectsPath, formName)
	if err != nil {
		return ""
	}

	for _, obj := range formObjects {
		if obj["fieldtype"] == "number" {
			if val, ok := obj["linked_table"]; ok {
				return val
			}
		}
	}

	return ""
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

	// look for dependent tables
	allTables := make([]string, 0)
	linkedToTables := make([]string, 0)

	for _, dirFI := range dirFIs {
		if !strings.HasSuffix(dirFI.Name(), ".f8p") {
			continue
		}

		allTables = append(allTables, strings.ReplaceAll(dirFI.Name(), ".f8p", ""))
		linkedToTable := getForeignKey(formObjectsPath, dirFI.Name())
		linkedToTable = strings.ReplaceAll(linkedToTable, ".f8p", "")

		if len(linkedToTable) != 0 {
			linkedToTables = append(linkedToTables, linkedToTable)
		}
	}

	// validating if all linked-to-tables exists
	for _, table := range linkedToTables {
		if slices.Index(allTables, table) != -1 {
			continue
		}

		tablesOnFlaarum, err := cl.ListTables()
		if err != nil {
			return F8Object{}, errors.Wrap(err, "flaarum error")
		}
		if slices.Index(tablesOnFlaarum, table) == -1 {
			return F8Object{}, errors.New(fmt.Sprintf("the linked-to-table '%s' is not on flaarum or list of form objects", table))
		}
	}

	// making sure linked-to-tables are created first
	lowerOrderTables := make([]string, 0)
	for _, table := range allTables {
		if slices.Index(linkedToTables, table) == -1 {
			lowerOrderTables = append(lowerOrderTables, table)
		}
	}

	for _, table := range append(linkedToTables, lowerOrderTables...) {
		stmt := getFlaarumStmt(formObjectsPath, table + ".f8p")
		err = cl.CreateOrUpdateTable(stmt)
		if err != nil {
			fmt.Println(err)
			return F8Object{formObjectsPath, cl}, errors.Wrap(err, "flaarum error")
		}		
	}


	return F8Object{formObjectsPath, cl}, nil
}

func (f8o *F8Object) GetNewForm(formName string) (string, error) {

	formObjects, err := getFormObjects(f8o.FormsObjectPath, formName)
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
	formObjects, err := getFormObjects(f8o.FormsObjectPath, formName)
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

func (f8o *F8Object) GetSubmittedData(r *http.Request, formName string) (map[string]string, error) {
	formObjects, err := getFormObjects(f8o.FormsObjectPath, formName)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]string)
	for _, obj := range formObjects {
		tmpValue := r.FormValue(obj["name"])
		isRequired :=  slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1
		if isRequired && len(tmpValue) == 0 {
			return nil, errors.New(fmt.Sprintf("field %s is required.", obj["fieldtype"]))
		}

		ret[ obj["name"] ] = tmpValue
	}

	return ret, nil
} 