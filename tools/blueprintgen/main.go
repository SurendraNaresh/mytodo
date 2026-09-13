// Command blueprintgen turns an approved blueprint into reviewable source.
// It intentionally generates typed tables and fixed SQL; it is not a runtime
// schema editor.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

type Blueprint struct {
	Modules []Module `json:"modules"`
}

type Module struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	Master  Entity `json:"master"`
	Detail  Entity `json:"detail"`
}

type Entity struct {
	Name   string   `json:"name"`
	Label  string   `json:"label"`
	Fields []Field  `json:"fields"`
	Unique []string `json:"unique"`
}

type Field struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func main() {
	blueprintPath := flag.String("blueprint", "blueprint.json", "blueprint JSON path")
	outputDir := flag.String("out", "generated", "output directory")
	flag.Parse()

	blueprint, err := load(*blueprintPath)
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fatal(err)
	}
	for _, module := range blueprint.Modules {
		if err := generate(module, *outputDir); err != nil {
			fatal(err)
		}
	}
}

func load(path string) (Blueprint, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Blueprint{}, err
	}
	var blueprint Blueprint
	if err := json.Unmarshal(content, &blueprint); err != nil {
		return Blueprint{}, fmt.Errorf("parse blueprint: %w", err)
	}
	if len(blueprint.Modules) == 0 {
		return Blueprint{}, errors.New("blueprint must contain at least one module")
	}
	for _, module := range blueprint.Modules {
		if !identifier.MatchString(module.Name) || !identifier.MatchString(module.Package) {
			return Blueprint{}, fmt.Errorf("invalid module name/package %q", module.Name)
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
	}
	for _, field := range entity.Unique {
		if !seen[field] {
			return fmt.Errorf("unique field %q is not defined", field)
		}
	}
	return nil
}

func generate(module Module, root string) error {
	dir := filepath.Join(root, module.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data := struct {
		Module
		MasterColumns string
		DetailColumns string
	}{Module: module, MasterColumns: columns(module.Master.Fields), DetailColumns: detailColumns(module)}

	if err := writeTemplate(filepath.Join(dir, module.Name+".sql"), sqlTemplate, data); err != nil {
		return err
	}
	if err := writeGo(filepath.Join(dir, module.Name+"_model.go"), modelTemplate, data); err != nil {
		return err
	}
	if err := writeGo(filepath.Join(dir, module.Name+"_controller.go"), controllerTemplate, data); err != nil {
		return err
	}
	return writeGo(filepath.Join(dir, module.Name+"_view.go"), viewTemplate, data)
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

func goName(name string) string {
	parts := strings.Split(name, "_")
	for index, part := range parts {
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func writeTemplate(path, source string, data any) error {
	tmpl, err := template.New(filepath.Base(path)).Parse(source)
	if err != nil {
		return err
	}
	var output strings.Builder
	if err := tmpl.Execute(&output, data); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(output.String()), 0644)
}

func writeGo(path, source string, data any) error {
	tmpl, err := template.New(filepath.Base(path)).Funcs(template.FuncMap{"goName": goName, "goType": goType}).Parse(source)
	if err != nil {
		return err
	}
	var output strings.Builder
	if err := tmpl.Execute(&output, data); err != nil {
		return err
	}
	formatted, err := format.Source([]byte(output.String()))
	if err != nil {
		return fmt.Errorf("format %s: %w", path, err)
	}
	return os.WriteFile(path, formatted, 0644)
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
	{{goName .Name}} {{goType .Type}}
{{- end}}
}

type {{goName .Detail.Name}} struct {
	ID int64
	{{goName .Master.Name}}ID int64
{{- range .Detail.Fields}}
	{{goName .Name}} {{goType .Type}}
{{- end}}
}
`

const controllerTemplate = `package {{.Package}}

import (
	"context"
	"database/sql"
)

// Create{{goName .Detail.Name}} uses fixed SQL and parameter placeholders.
func Create{{goName .Detail.Name}}(ctx context.Context, db *sql.DB, parentID int64, values ...any) (int64, error) {
	result, err := db.ExecContext(ctx, "INSERT INTO {{.Detail.Name}} ({{.Master.Name}}_id{{range .Detail.Fields}}, {{.Name}}{{end}}) VALUES (?{{range .Detail.Fields}}, ?{{end}})", append([]any{parentID}, values...)...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
`

const viewTemplate = `package {{.Package}}

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// New{{goName .Master.Name}}View is a deliberately small generated starting point.
// Add domain-specific layout and validation in a hand-written companion file.
func New{{goName .Master.Name}}View() fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, nil, widget.NewLabel("{{.Master.Label}}"))
}
`

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
