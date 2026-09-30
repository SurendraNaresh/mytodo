// Command blueprintgen turns an approved blueprint into reviewable source.
// It intentionally generates typed tables and fixed SQL; it is not a runtime
// schema editor.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type Blueprint struct {
	Modules []Module `json:"modules"`
	Types   []Type   `json:"types"`
}

type Type struct {
	TypeKey      string            `json:"type_key"`
	TypeID       int64             `json:"type_id"`
	ParentTypeID *int64            `json:"parent_type_id"`
	Header       map[string]string `json:"header"`
	LogicRule    LogicRule         `json:"logic_rule"`
	UI           TypeUI            `json:"ui"`
}

type LogicRule struct {
	Validation string `json:"validation"`
}

type TypeUI struct {
	ListCols  []string `json:"list_cols"`
	FormOrder []string `json:"form_order"`
}

type Module struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	Label   string `json:"label"`
	Master  Entity `json:"master"`
	Detail  Entity `json:"detail"`
}

type Entity struct {
	Name            string           `json:"name"`
	Label           string           `json:"label"`
	Fields          []Field          `json:"fields"`
	Unique          []string         `json:"unique"`
	ValidationRules []ValidationRule `json:"validation_rules"`
}

type Field struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options"`
	RangeMin string   `json:"range_min"`
	RangeMax string   `json:"range_max"`
}

type ValidationRule struct {
	When     map[string]string `json:"when"`
	Required []string          `json:"required"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

func main() {
	blueprintPath := flag.String("blueprint", "blueprint.json", "blueprint JSON path")
	outputDir := flag.String("out", "generated", "output directory")
	flag.Parse()

	blueprint, err := load(*blueprintPath)
	if err != nil {
		fatal(err)
	}
	if err := generateBlueprint(blueprint, *outputDir); err != nil {
		fatal(err)
	}
}

func load(path string) (Blueprint, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Blueprint{}, err
	}
	var blueprint Blueprint
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&blueprint); err != nil {
		return Blueprint{}, fmt.Errorf("parse blueprint: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Blueprint{}, errors.New("parse blueprint: multiple JSON values")
		}
		return Blueprint{}, fmt.Errorf("parse blueprint: %w", err)
	}
	if len(blueprint.Modules) == 0 {
		return Blueprint{}, errors.New("blueprint must contain at least one module")
	}
	seen := map[string]bool{}
	for _, module := range blueprint.Modules {
		if !identifier.MatchString(module.Name) || !identifier.MatchString(module.Package) {
			return Blueprint{}, fmt.Errorf("invalid module name/package %q", module.Name)
		}
		for _, name := range []string{module.Name, module.Master.Name, module.Detail.Name} {
			if seen[name] {
				return Blueprint{}, fmt.Errorf("duplicate id %q across module", name)
			}
			seen[name] = true
		}
		if err := validateEntity(module.Master); err != nil {
			return Blueprint{}, fmt.Errorf("%s master: %w", module.Name, err)
		}
		if err := validateEntity(module.Detail); err != nil {
			return Blueprint{}, fmt.Errorf("%s detail: %w", module.Name, err)
		}
	}
	return blueprint, nil
}

func validateEntity(entity Entity) error {
	if !identifier.MatchString(entity.Name) || len(entity.Fields) == 0 {
		return errors.New("entity name and fields are required")
	}
	validTypes := map[string]bool{"text": true, "int64": true, "bool": true, "datetime": true}
	seen := map[string]bool{}
	for _, field := range entity.Fields {
		if !identifier.MatchString(field.Name) || !validTypes[field.Type] || seen[field.Name] {
			return fmt.Errorf("invalid or duplicate field %q", field.Name)
		}
		seen[field.Name] = true
		if len(field.Options) > 0 && field.Type != "text" {
			return fmt.Errorf("options are only supported for text field %q", field.Name)
		}
		optionValues := map[string]bool{}
		for _, option := range field.Options {
			if strings.TrimSpace(option) == "" || optionValues[option] {
				return fmt.Errorf("invalid or duplicate option for field %q", field.Name)
			}
			optionValues[option] = true
		}
		if (field.RangeMin == "") != (field.RangeMax == "") {
			return fmt.Errorf("field %q must define both range_min and range_max", field.Name)
		}
		if field.RangeMin != "" {
			switch field.Type {
			case "int64":
				minimum, minErr := strconv.ParseInt(field.RangeMin, 10, 64)
				maximum, maxErr := strconv.ParseInt(field.RangeMax, 10, 64)
				if minErr != nil || maxErr != nil || minimum > maximum {
					return fmt.Errorf("invalid range for field %q", field.Name)
				}
			case "datetime":
				minimum, minErr := time.Parse("15:04", field.RangeMin)
				maximum, maxErr := time.Parse("15:04", field.RangeMax)
				if minErr != nil || maxErr != nil || minimum.After(maximum) {
					return fmt.Errorf("invalid time range for field %q", field.Name)
				}
			default:
				return fmt.Errorf("ranges are not supported for field %q of type %q", field.Name, field.Type)
			}
		}
	}
	for _, field := range entity.Unique {
		if !seen[field] {
			return fmt.Errorf("unique field %q is not defined", field)
		}
	}
	for _, rule := range entity.ValidationRules {
		if len(rule.When) == 0 || len(rule.Required) == 0 {
			return errors.New("validation rule must define when and required")
		}
		for field, value := range rule.When {
			if !seen[field] {
				return fmt.Errorf("validation condition field %q is not defined", field)
			}
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("validation condition for field %q has an empty value", field)
			}
		}
		for _, field := range rule.Required {
			if !seen[field] {
				return fmt.Errorf("validation required field %q is not defined", field)
			}
		}
	}
	return nil
}

func columns(fields []Field) string {
	parts := []string{"id INTEGER PRIMARY KEY AUTOINCREMENT"}
	for _, field := range fields {
		nullability := ""
		if field.Required {
			nullability = " NOT NULL"
		}
		parts = append(parts, fmt.Sprintf("%s %s%s", field.Name, sqlType(field.Type), nullability))
	}
	return strings.Join(parts, ",\n    ")
}

func masterColumns(entity Entity) string {
	parts := []string{columns(entity.Fields)}
	if len(entity.Unique) > 0 {
		unique := append([]string(nil), entity.Unique...)
		sort.Strings(unique)
		parts = append(parts, "UNIQUE ("+strings.Join(unique, ", ")+")")
	}
	return strings.Join(parts, ",\n    ")
}

func detailColumns(module Module) string {
	parts := []string{fmt.Sprintf("%s_id INTEGER NOT NULL REFERENCES %s(id) ON DELETE CASCADE", module.Master.Name, module.Master.Name)}
	for _, field := range module.Detail.Fields {
		nullability := ""
		if field.Required {
			nullability = " NOT NULL"
		}
		parts = append(parts, fmt.Sprintf("%s %s%s", field.Name, sqlType(field.Type), nullability))
	}
	if len(module.Detail.Unique) > 0 {
		unique := append([]string(nil), module.Detail.Unique...)
		sort.Strings(unique)
		unique = append([]string{module.Master.Name + "_id"}, unique...)
		parts = append(parts, "UNIQUE ("+strings.Join(unique, ", ")+")")
	}
	return strings.Join(parts, ",\n    ")
}

func sqlType(fieldType string) string {
	if fieldType == "int64" || fieldType == "bool" {
		return "INTEGER"
	}
	return "TEXT"
}

func goType(fieldType string) string {
	if fieldType == "int64" {
		return "int64"
	}
	if fieldType == "bool" {
		return "bool"
	}
	return "string"
}

func modelFieldType(field Field) string {
	typ := goType(field.Type)
	if !field.Required {
		return "*" + typ
	}
	return typ
}

func goName(name string) string {
	parts := strings.Split(name, "_")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		result = append(result, strings.ToUpper(part[:1])+part[1:])
	}
	return strings.Join(result, "")
}

func goVariable(name string) string {
	value := goName(name)
	if value == "" {
		return ""
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func renderBlueprint(blueprint Blueprint) (map[string][]byte, error) {
	outputs := map[string][]byte{}
	for _, module := range blueprint.Modules {
		files, err := renderModule(module)
		if err != nil {
			return nil, err
		}
		for path, content := range files {
			outputs[filepath.Join(module.Name, path)] = content
		}
	}
	registry, err := renderGo("modules.go", moduleTabsTemplate, blueprint)
	if err != nil {
		return nil, err
	}
	outputs["modules.go"] = registry
	return outputs, nil
}

func generateBlueprint(blueprint Blueprint, root string) error {
	outputs, err := renderBlueprint(blueprint)
	if err != nil {
		return err
	}
	return writeOutputs(outputs, root)
}

func renderModule(module Module) (map[string][]byte, error) {
	data := struct {
		Module
		MasterColumns string
		DetailColumns string
	}{Module: module, MasterColumns: masterColumns(module.Master), DetailColumns: detailColumns(module)}
	files := map[string][]byte{}
	for name, source := range map[string]string{
		module.Name + ".sql":           sqlTemplate,
		module.Name + "_model.go":      modelTemplate,
		module.Name + "_controller.go": controllerTemplate,
		module.Name + "_view.go":       viewTemplate,
	} {
		var content []byte
		var err error
		if filepath.Ext(name) == ".go" {
			content, err = renderGo(name, source, data)
		} else {
			content, err = renderTemplate(name, source, data)
		}
		if err != nil {
			return nil, err
		}
		files[name] = content
	}
	return files, nil
}

func renderTemplate(name, source string, data any) ([]byte, error) {
	tmpl, err := template.New(name).Funcs(template.FuncMap{
		"columns":      fieldColumns,
		"placeholders": placeholders,
		"assignments":  assignments,
	}).Parse(source)
	if err != nil {
		return nil, err
	}
	var output strings.Builder
	if err := tmpl.Execute(&output, data); err != nil {
		return nil, err
	}
	return []byte(output.String()), nil
}

func renderGo(name, source string, data any) ([]byte, error) {
	tmpl, err := template.New(name).Funcs(template.FuncMap{
		"goName":       goName,
		"goVariable":   goVariable,
		"goType":       goType,
		"modelType":    modelFieldType,
		"plural":       pluralGoName,
		"columns":      fieldColumns,
		"placeholders": placeholders,
		"assignments":  assignments,
	}).Parse(source)
	if err != nil {
		return nil, err
	}
	var output strings.Builder
	if err := tmpl.Execute(&output, data); err != nil {
		return nil, err
	}
	formatted, err := format.Source([]byte(output.String()))
	if err != nil {
		return nil, fmt.Errorf("format %s: %w", name, err)
	}
	return formatted, nil
}

func fieldColumns(fields []Field) string {
	columns := make([]string, len(fields))
	for index, field := range fields {
		columns[index] = field.Name
	}
	return strings.Join(columns, ", ")
}

func placeholders(fields []Field) string {
	return strings.TrimSuffix(strings.Repeat("?, ", len(fields)), ", ")
}

func assignments(fields []Field) string {
	assignments := make([]string, len(fields))
	for index, field := range fields {
		assignments[index] = field.Name + " = ?"
	}
	return strings.Join(assignments, ", ")
}

func pluralGoName(name string) string {
	if strings.HasSuffix(name, "y") {
		return strings.TrimSuffix(name, "y") + "ies"
	}
	if strings.HasSuffix(name, "s") {
		return name + "es"
	}
	return name + "s"
}

type stagedOutput struct {
	destination string
	temporary   string
	backup      string
	installed   bool
}

func writeOutputs(outputs map[string][]byte, root string) error {
	paths := make([]string, 0, len(outputs))
	for path := range outputs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	staged := make([]stagedOutput, 0, len(paths))
	cleanup := func() {
		for _, file := range staged {
			_ = os.Remove(file.temporary)
			if file.backup != "" {
				_ = os.Remove(file.backup)
			}
		}
	}
	for _, path := range paths {
		destination := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			cleanup()
			return err
		}
		temporary, err := os.CreateTemp(filepath.Dir(destination), ".blueprintgen-stage-*")
		if err != nil {
			cleanup()
			return err
		}
		if _, err := temporary.Write(outputs[path]); err != nil {
			temporary.Close()
			os.Remove(temporary.Name())
			cleanup()
			return err
		}
		if err := temporary.Chmod(0644); err != nil {
			temporary.Close()
			os.Remove(temporary.Name())
			cleanup()
			return err
		}
		if err := temporary.Close(); err != nil {
			os.Remove(temporary.Name())
			cleanup()
			return err
		}
		staged = append(staged, stagedOutput{destination: destination, temporary: temporary.Name()})
	}
	for index := range staged {
		file := &staged[index]
		if _, err := os.Stat(file.destination); err == nil {
			backup, createErr := os.CreateTemp(filepath.Dir(file.destination), ".blueprintgen-backup-*")
			if createErr != nil {
				rollbackOutputs(staged[:index])
				cleanup()
				return createErr
			}
			file.backup = backup.Name()
			if err := backup.Close(); err != nil {
				rollbackOutputs(staged[:index])
				cleanup()
				return err
			}
			if err := os.Remove(file.backup); err != nil {
				rollbackOutputs(staged[:index])
				cleanup()
				return err
			}
			if err := os.Rename(file.destination, file.backup); err != nil {
				file.backup = ""
				rollbackOutputs(staged[:index])
				cleanup()
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			rollbackOutputs(staged[:index])
			cleanup()
			return err
		}
		if err := os.Rename(file.temporary, file.destination); err != nil {
			if file.backup != "" {
				_ = os.Rename(file.backup, file.destination)
				file.backup = ""
			}
			rollbackOutputs(staged[:index])
			cleanup()
			return err
		}
		file.installed = true
	}
	cleanup()
	return nil
}

func rollbackOutputs(files []stagedOutput) {
	for index := len(files) - 1; index >= 0; index-- {
		file := files[index]
		if file.installed {
			_ = os.Remove(file.destination)
		}
		if file.backup != "" {
			_ = os.Rename(file.backup, file.destination)
		}
	}
}

const sqlTemplate = `-- Generated from blueprint. Review before applying.
CREATE TABLE IF NOT EXISTS {{.Master.Name}} (
    {{.MasterColumns}}
);

CREATE TABLE IF NOT EXISTS {{.Detail.Name}} (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
	{{.DetailColumns}}
);
CREATE INDEX IF NOT EXISTS ix_{{.Detail.Name}}_parent ON {{.Detail.Name}}({{.Master.Name}}_id);
`

const modelTemplate = `package {{.Package}}

type {{goName .Master.Name}} struct {
	ID int64
{{- range .Master.Fields}}
	{{goName .Name}} {{modelType .}}
{{- end}}
}

type {{goName .Detail.Name}} struct {
	ID int64
	{{goName .Master.Name}}ID int64
{{- range .Detail.Fields}}
	{{goName .Name}} {{modelType .}}
{{- end}}
}
`

const controllerTemplate = `package {{.Package}}

import (
	"context"
	"database/sql"
)

func Create{{goName .Master.Name}}(ctx context.Context, db *sql.DB, values ...any) (int64, error) {
	result, err := db.ExecContext(ctx, "INSERT INTO {{.Master.Name}} ({{columns .Master.Fields}}) VALUES ({{placeholders .Master.Fields}})", values...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func Get{{goName .Master.Name}}(ctx context.Context, db *sql.DB, id int64) ({{goName .Master.Name}}, error) {
	var value {{goName .Master.Name}}
	err := db.QueryRowContext(ctx, "SELECT id{{range .Master.Fields}}, {{.Name}}{{end}} FROM {{.Master.Name}} WHERE id = ?", id).Scan(&value.ID{{range .Master.Fields}}, &value.{{goName .Name}}{{end}})
	return value, err
}

func List{{plural (goName .Master.Name)}}(ctx context.Context, db *sql.DB) ([]{{goName .Master.Name}}, error) {
	rows, err := db.QueryContext(ctx, "SELECT id{{range .Master.Fields}}, {{.Name}}{{end}} FROM {{.Master.Name}}")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []{{goName .Master.Name}}
	for rows.Next() {
		var value {{goName .Master.Name}}
		if err := rows.Scan(&value.ID{{range .Master.Fields}}, &value.{{goName .Name}}{{end}}); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func Update{{goName .Master.Name}}(ctx context.Context, db *sql.DB, id int64, values ...any) error {
	args := append(append([]any(nil), values...), id)
	_, err := db.ExecContext(ctx, "UPDATE {{.Master.Name}} SET {{assignments .Master.Fields}} WHERE id = ?", args...)
	return err
}

func Delete{{goName .Master.Name}}(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM {{.Master.Name}} WHERE id = ?", id)
	return err
}

func Create{{goName .Detail.Name}}(ctx context.Context, db *sql.DB, parentID int64, values ...any) (int64, error) {
	result, err := db.ExecContext(ctx, "INSERT INTO {{.Detail.Name}} ({{.Master.Name}}_id, {{columns .Detail.Fields}}) VALUES (?, {{placeholders .Detail.Fields}})", append([]any{parentID}, values...)...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func Get{{goName .Detail.Name}}(ctx context.Context, db *sql.DB, id int64) ({{goName .Detail.Name}}, error) {
	var value {{goName .Detail.Name}}
	err := db.QueryRowContext(ctx, "SELECT id, {{.Master.Name}}_id{{range .Detail.Fields}}, {{.Name}}{{end}} FROM {{.Detail.Name}} WHERE id = ?", id).Scan(&value.ID, &value.{{goName .Master.Name}}ID{{range .Detail.Fields}}, &value.{{goName .Name}}{{end}})
	return value, err
}

func List{{plural (goName .Detail.Name)}}By{{goName .Master.Name}}(ctx context.Context, db *sql.DB, parentID int64) ([]{{goName .Detail.Name}}, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, {{.Master.Name}}_id{{range .Detail.Fields}}, {{.Name}}{{end}} FROM {{.Detail.Name}} WHERE {{.Master.Name}}_id = ?", parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []{{goName .Detail.Name}}
	for rows.Next() {
		var value {{goName .Detail.Name}}
		if err := rows.Scan(&value.ID, &value.{{goName .Master.Name}}ID{{range .Detail.Fields}}, &value.{{goName .Name}}{{end}}); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func Update{{goName .Detail.Name}}(ctx context.Context, db *sql.DB, id int64, values ...any) error {
	args := append(append([]any(nil), values...), id)
	_, err := db.ExecContext(ctx, "UPDATE {{.Detail.Name}} SET {{assignments .Detail.Fields}} WHERE id = ?", args...)
	return err
}

func Delete{{goName .Detail.Name}}(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM {{.Detail.Name}} WHERE id = ?", id)
	return err
}
`

const viewTemplate = `package {{.Package}}

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func New{{goName .Master.Name}}View() fyne.CanvasObject {
	{{range .Master.Fields}}
	{{if eq .Type "datetime"}}
	{{goVariable .Name}}Date := widget.NewDateEntry()
	{{goVariable .Name}}Date.SetPlaceHolder("YYYY-MM-DD")
	{{goVariable .Name}}Time := widget.NewEntry()
	{{goVariable .Name}}Time.SetPlaceHolder("HH:MM")
	{{else}}
	{{goVariable .Name}} := {{if eq .Type "bool"}}widget.NewCheck("", nil){{else}}widget.NewEntry(){{end}}
	{{end}}
	{{end}}
	form := widget.NewForm(
	{{range .Master.Fields}}
		{{if eq .Type "datetime"}}
		widget.NewFormItem("{{.Label}}", container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Date"), {{goVariable .Name}}Date),
			container.NewVBox(widget.NewLabel("Time"), {{goVariable .Name}}Time),
		)),
		{{else}}
		widget.NewFormItem("{{.Label}}", {{goVariable .Name}}),
		{{end}}
	{{end}}
	)
	actions := container.NewHBox(
		widget.NewButton("Save", nil),
		widget.NewButton("Edit", nil),
		widget.NewButton("Add-New", nil),
	)
	return container.NewBorder(actions, nil, nil, nil, container.NewVScroll(form))
}
`

const moduleTabsTemplate = `package generated

import (
	"fyne.io/fyne/v2/container"
{{- range $index, $module := .Modules}}
	module{{$index}} "github.com/SurendraNaresh/mytodo/generated/{{$module.Name}}"
{{- end}}
)

func ModuleTabs() []*container.TabItem {
	return []*container.TabItem{
{{- range $index, $module := .Modules}}
		container.NewTabItem("{{if $module.Label}}{{$module.Label}}{{else}}{{$module.Detail.Name}}{{end}}", module{{$index}}.New{{goName $module.Master.Name}}View()),
{{- end}}
	}
}
`

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
