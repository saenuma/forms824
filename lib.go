package forms824

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
		} else if obj["fieldtype"] == "int" {
			flaarumField = "int"

			if val, ok := obj["linked_table"]; ok && len(val) != 0 {
				hasForeignKeys = true
				stmtSuffix += fmt.Sprintf("%s %s on_delete_delete \n", obj["name"], val)
			}

		} else if obj["fieldtype"] == "text" {
			flaarumField = "text"
		} else if obj["fieldtype"] == "float" {
			flaarumField = "float"
		}

		attribs := strings.Split(obj["attributes"], ";")
		if slices.Index(attribs, "hidden") != -1 {
			hiddenAttribIndex := slices.Index(attribs, "hidden")
			attribs = slices.Delete(attribs, hiddenAttribIndex, hiddenAttribIndex+1)
		}
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
		if obj["fieldtype"] == "int" {
			if val, ok := obj["linked_table"]; ok {
				return val
			}
		}
	}

	return ""
}

type F8Object struct {
	FormsPath     string
	FlaarumClient flaarum.Client
}

// creates tables for all formObjects and returns a struct
// this must be ran before using any other function in this library.
func Init(formObjectsPath string, cl flaarum.Client) (F8Object, error) {
	if !doesPathExists(formObjectsPath) {
		return F8Object{}, errors.New(fmt.Sprintf("FormsPath %s does not exists.", formObjectsPath))
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
		stmt := getFlaarumStmt(formObjectsPath, table+".f8p")
		err = cl.CreateOrUpdateTable(stmt)
		if err != nil {
			fmt.Println(err)
			return F8Object{formObjectsPath, cl}, errors.Wrap(err, "flaarum error")
		}
	}

	return F8Object{formObjectsPath, cl}, nil
}

func (f8o *F8Object) ListForms() ([]string, error) {
	dirFIs, err := os.ReadDir(f8o.FormsPath)
	if err != nil {
		return []string{}, errors.Wrap(err, "os error")
	}

	allTables := make([]string, 0)
	for _, dirFI := range dirFIs {
		if !strings.HasSuffix(dirFI.Name(), ".f8p") {
			continue
		}

		allTables = append(allTables, strings.ReplaceAll(dirFI.Name(), ".f8p", ""))
	}
	return allTables, nil
}

func (f8o *F8Object) GetNewForm(formName string) (string, error) {

	formObjects, err := getFormObjects(f8o.FormsPath, formName)
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

		if obj["fieldtype"] == "int" {
			html += fmt.Sprintf("<input type='number' name='%s' id='id_%s' min='%s' max='%s' ",
				obj["name"], obj["name"], obj["min_value"], obj["max_value"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += "/>"
		} else if obj["fieldtype"] == "float" {
			html += fmt.Sprintf("<input type='number' name='%s' id='id_%s' min='%s' max='%s' step='0.0001'",
				obj["name"], obj["name"], obj["min_value"], obj["max_value"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += "/>"

		} else if slices.Index([]string{"string", "email", "date", "datetime"}, obj["fieldtype"]) != -1 {
			fieldType := obj["fieldtype"]
			if fieldType == "datetime" {
				fieldType += "-local"
			}
			if fieldType == "string" {
				fieldType = "text"
			}
			html += fmt.Sprintf("<input type='%s' name='%s' id='id_%s' ", fieldType,
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
		} else if obj["fieldtype"] == "multi_display_select" {
			html += "<div>"
			for _, opt := range strings.Split(obj["select_options"], "\n") {
				html += fmt.Sprintf("<input type='checkbox' id='id_%s' name='%s' value='%s' /> %s", obj["name"],
					obj["name"], opt, opt)
			}
			html += "</div>"
		} else if obj["fieldtype"] == "single_display_select" {
			html += "<div>"
			for _, opt := range strings.Split(obj["select_options"], "\n") {
				html += fmt.Sprintf("<input type='radio' id='id_%s' name='%s' value='%s' /> %s", obj["name"],
					obj["name"], opt, opt)
			}
			html += "</div>"
		} else if obj["fieldtype"] == "text" {
			html += fmt.Sprintf("<textarea id='id_%s' name='%s'", obj["name"], obj["name"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += "></textarea>"
		} else if obj["fieldtype"] == "check" {
			html += fmt.Sprintf("<input type='checkbox' id='id_%s' name='%s' /> %s", obj["name"],
				obj["name"], obj["label"])
		}
		html += "</div>"
	}

	return html, nil
}

func (f8o *F8Object) GetEditForm(formName string, dataId int64) (string, error) {
	formObjects, err := getFormObjects(f8o.FormsPath, formName)
	if err != nil {
		return "", err
	}

	oldData, err := f8o.FlaarumClient.SearchForOne(fmt.Sprintf(`
		table: %s
		where:
			id = %d
		`, formName, dataId))
	if err != nil {
		return "", errors.Wrap(err, "flaarum error")
	}

	var html string
	for _, obj := range formObjects {
		if slices.Index(strings.Split(obj["attributes"], ";"), "hidden") != -1 {
			continue
		}
		var currentOldData string
		tmpValue, ok := (*oldData)[obj["name"]]
		if ok {
			switch val := tmpValue.(type) {
			case int64:
				currentOldData = strconv.FormatInt(val, 10)
			case string:
				currentOldData = val
			}
		}

		html += "<div>"
		html += fmt.Sprintf("<div><label for='id_%s'>%s</label></div>", obj["name"], obj["label"])
		if obj["fieldtype"] == "int" {
			html += fmt.Sprintf("<input type='number' name='%s' id='id_%s' min='%s' max='%s' value='%s'",
				obj["name"], obj["name"], obj["min_value"], obj["max_value"], currentOldData)
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += "/>"
		} else if slices.Index([]string{"string", "email", "date", "datetime"}, obj["fieldtype"]) != -1 {
			fieldType := obj["fieldtype"]
			if fieldType == "datetime" {
				fieldType += "-local"
			}
			if fieldType == "string" {
				fieldType = "text"
			}
			html += fmt.Sprintf("<input type='%s' name='%s' id='id_%s' value='%s'", fieldType,
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

		} else if obj["fieldtype"] == "multi_display_select" {
			html += "<div>"
			for _, opt := range strings.Split(obj["select_options"], "\n") {
				pickedChoices := strings.Split(currentOldData, ";")
				if slices.Index(pickedChoices, opt) != -1 {
					html += fmt.Sprintf("<input type='checkbox' id='id_%s' name='%s' value='%s' checked /> %s", obj["name"],
						obj["name"], opt, opt)

				} else {
					html += fmt.Sprintf("<input type='checkbox' id='id_%s' name='%s' value='%s' /> %s", obj["name"],
						obj["name"], opt, opt)
				}
			}
			html += "</div>"
		} else if obj["fieldtype"] == "single_display_select" {
			html += "<div>"
			for _, opt := range strings.Split(obj["select_options"], "\n") {
				if opt == currentOldData {
					html += fmt.Sprintf("<input type='radio' id='id_%s' name='%s' value='%s' checked /> %s", obj["name"],
						obj["name"], opt, opt)
				} else {
					html += fmt.Sprintf("<input type='radio' id='id_%s' name='%s' value='%s' /> %s", obj["name"],
						obj["name"], opt, opt)
				}

			}
			html += "</div>"

		} else if obj["fieldtype"] == "text" {
			html += fmt.Sprintf("<textarea id='id_%s' name='%s'", obj["name"], obj["name"])
			if slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1 {
				html += " required"
			}
			html += ">" + currentOldData + "</textarea>"
		} else if obj["fieldtype"] == "check" {
			checkedStr := ""
			if currentOldData == "on" || currentOldData == "true" || currentOldData == "yes" {
				checkedStr = "checked"
			}
			html += fmt.Sprintf("<input type='checkbox' id='id_%s' name='%s' %s/> %s", obj["name"],
				obj["name"], checkedStr, obj["label"])
		}
		html += "</div>"
	}

	return html, nil
}

// multi_display_select returns a string joined by ';' because this field can have a list as its value
func (f8o *F8Object) GetSubmittedData(r *http.Request, formName string) (map[string]string, error) {
	formObjects, err := getFormObjects(f8o.FormsPath, formName)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]string)
	for _, obj := range formObjects {
		tmpValue := r.FormValue(obj["name"])
		isRequired := slices.Index(strings.Split(obj["attributes"], ";"), "required") != -1
		if isRequired && len(tmpValue) == 0 {
			return nil, errors.New(fmt.Sprintf("field %s is required.", obj["fieldtype"]))
		}

		if obj["fieldtype"] == "multi_display_select" {
			r.ParseForm()
			ret[obj["name"]] = strings.Join(r.Form[obj["name"]], ";")
		} else {
			ret[obj["name"]] = tmpValue
		}
	}

	return ret, nil
}
