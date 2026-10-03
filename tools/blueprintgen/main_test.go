package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testBlueprint() Blueprint {
	return Blueprint{
		Modules: []Module{{
			Name:    "voting",
			Package: "voting",
			Label:   "Event",
			Master: Entity{
				Name:   "voting_event",
				Unique: []string{"title"},
				Fields: []Field{
					{Name: "title", Label: "Title", Type: "text", Required: true},
					{Name: "description", Label: "Description", Type: "text"},
					{Name: "opens_at", Label: "Opens", Type: "datetime", Required: true, RangeMin: "06:00", RangeMax: "22:00"},
					{Name: "closes_at", Label: "Closes", Type: "datetime", Required: true},
				},
			},
			Detail: Entity{
				Name: "vote",
				Fields: []Field{
					{Name: "voter_user_id", Type: "int64", Required: true},
					{Name: "choice", Type: "text", Required: true, Options: []string{"Yes", "No", "Abstain"}},
					{Name: "comments", Type: "text"},
				},
				ValidationRules: []ValidationRule{{When: map[string]string{"choice": "Abstain"}, Required: []string{"comments"}}},
			},
		}},
		Types: []Type{{
			TypeKey:      "manager",
			TypeID:       101,
			ParentTypeID: nil,
			Header:       map[string]string{"ndc_1": "Name"},
			LogicRule:    LogicRule{Validation: "fnValidateManager"},
			UI:           TypeUI{ListCols: []string{"ndc_1"}, FormOrder: []string{"ndc_1"}},
		}},
	}
}

func TestValidateEntityRangesAndConditionalRequiredFields(t *testing.T) {
	blueprint := testBlueprint()
	if _, err := load(writeBlueprint(t, blueprint)); err != nil {
		t.Fatalf("valid ranges and conditional rule rejected: %v", err)
	}

	blueprint.Modules[0].Detail.ValidationRules[0].Required = []string{"missing"}
	if _, err := load(writeBlueprint(t, blueprint)); err == nil || !strings.Contains(err.Error(), `validation required field "missing" is not defined`) {
		t.Fatalf("expected undefined conditional field rejection, got %v", err)
	}

	blueprint = testBlueprint()
	blueprint.Modules[0].Master.Fields[2].RangeMax = "05:00"
	if _, err := load(writeBlueprint(t, blueprint)); err == nil || !strings.Contains(err.Error(), "invalid time range") {
		t.Fatalf("expected reversed time range rejection, got %v", err)
	}
}

func writeBlueprint(t *testing.T, blueprint Blueprint) string {
	t.Helper()
	content, err := json.Marshal(blueprint)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "blueprint.json")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAcceptsTypesAndRejectsUnknownKeys(t *testing.T) {
	path := writeBlueprint(t, testBlueprint())
	loaded, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Types) != 1 || loaded.Types[0].TypeKey != "manager" || loaded.Types[0].Header["ndc_1"] != "Name" {
		t.Fatalf("types block was not retained: %#v", loaded.Types)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatal(err)
	}
	document["unknown"] = true
	content, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown key rejection, got %v", err)
	}
}

func TestIdentifierRejectsEmptySegments(t *testing.T) {
	for _, value := range []string{"a__b", "a_", "_a"} {
		if identifier.MatchString(value) {
			t.Errorf("identifier unexpectedly accepted %q", value)
		}
	}
	if got := goName(""); got != "" {
		t.Fatalf("goName empty = %q, want empty", got)
	}
	if got := goName("a__b"); got != "AB" {
		t.Fatalf("goName with empty segment = %q, want AB", got)
	}

	for _, invalid := range []string{"a__b", "bad_"} {
		blueprint := testBlueprint()
		blueprint.Modules[0].Master.Fields[0].Name = invalid
		if _, err := load(writeBlueprint(t, blueprint)); err == nil {
			t.Errorf("load accepted malformed identifier %q", invalid)
		}
	}
}

func TestLoadRejectsGlobalIDCollision(t *testing.T) {
	blueprint := testBlueprint()
	blueprint.Modules = append(blueprint.Modules, Module{
		Name:    "ballots",
		Package: "ballots",
		Master:  Entity{Name: "vote", Fields: []Field{{Name: "title", Type: "text"}}},
		Detail:  Entity{Name: "ballot", Fields: []Field{{Name: "choice", Type: "text"}}},
	})
	if _, err := load(writeBlueprint(t, blueprint)); err == nil || !strings.Contains(err.Error(), `duplicate id "vote"`) {
		t.Fatalf("expected duplicate global ID rejection, got %v", err)
	}
}

func TestRenderIncludesMasterAndDetailCRUD(t *testing.T) {
	outputs, err := renderBlueprint(testBlueprint())
	if err != nil {
		t.Fatal(err)
	}
	controller := string(outputs[filepath.Join("voting", "voting_controller.go")])
	for _, function := range []string{
		"CreateVotingEvent", "GetVotingEvent", "ListVotingEvents", "UpdateVotingEvent", "DeleteVotingEvent",
		"CreateVote", "GetVote", "ListVotesByVotingEvent", "UpdateVote", "DeleteVote",
	} {
		if !strings.Contains(controller, "func "+function+"(") {
			t.Errorf("generated controller is missing %s", function)
		}
	}
	if !strings.Contains(string(outputs[filepath.Join("voting", "voting_model.go")]), "Description *string") {
		t.Fatal("optional fields should retain SQL NULL in generated models")
	}
	view := string(outputs[filepath.Join("voting", "voting_view.go")])
	for _, field := range []string{
		"widget.NewForm(",
		`widget.NewFormItem("Title", title)`,
		`widget.NewFormItem("Description", description)`,
		`widget.NewFormItem("Opens", container.NewGridWithColumns(2,`,
		`widget.NewFormItem("Closes", container.NewGridWithColumns(2,`,
		`opensAtDate := widget.NewDateEntry()`,
		`opensAtDate.SetPlaceHolder("YYYY-MM-DD")`,
		`opensAtTime.SetPlaceHolder("HH:MM")`,
		`closesAtDate := widget.NewDateEntry()`,
		`closesAtTime.SetPlaceHolder("HH:MM")`,
		`widget.NewButton("Save", nil)`,
		`widget.NewButton("Edit", nil)`,
		`widget.NewButton("Add-New", nil)`,
		`container.NewBorder(actions, nil, nil, nil, container.NewVScroll(form))`,
		"container.NewVScroll(form)",
	} {
		if !strings.Contains(view, field) {
			t.Errorf("generated view is missing %q", field)
		}
	}
	registry := string(outputs["modules.go"])
	if !strings.Contains(registry, `container.NewTabItem("Event"`) || !strings.Contains(registry, "NewVotingEventView()") {
		t.Fatalf("generated module tabs do not wire the Event view: %s", registry)
	}
	sql := string(outputs[filepath.Join("voting", "voting.sql")])
	if !strings.Contains(sql, "UNIQUE (title)") {
		t.Fatalf("generated master table is missing its unique constraint: %s", sql)
	}
}

func TestRenderStandaloneTablesIncludesConstraints(t *testing.T) {
	blueprint := testBlueprint()
	blueprint.Tables = []Table{
		{
			Name: "era",
			Fields: []Field{
				{Name: "slug", Type: "text", Required: true},
				{Name: "title", Type: "text", Required: true},
			},
			Unique: []string{"slug"},
		},
		{
			Name:        "album",
			Fields:      []Field{{Name: "era_id", Type: "int64", Required: true}},
			ForeignKeys: []ForeignKey{{Field: "era_id", References: "era.id", OnDelete: "CASCADE"}},
		},
		{
			Name:   "media",
			Fields: []Field{{Name: "kind", Type: "text", Required: true, Options: []string{"photo", "short"}}},
		},
	}
	if _, err := load(writeBlueprint(t, blueprint)); err != nil {
		t.Fatalf("valid standalone tables rejected: %v", err)
	}
	outputs, err := renderBlueprint(blueprint)
	if err != nil {
		t.Fatal(err)
	}
	schema := string(outputs["schema.sql"])
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS era",
		"UNIQUE (slug)",
		"FOREIGN KEY (era_id) REFERENCES era(id) ON DELETE CASCADE",
		"CHECK (kind IN ('photo', 'short'))",
	} {
		if !strings.Contains(schema, expected) {
			t.Errorf("generated schema is missing %q", expected)
		}
	}
	if !strings.Contains(string(outputs[filepath.Join("schema", "schema.go")]), "const EraSchemaSQL =") {
		t.Fatal("generated Go schema constant is missing")
	}

	blueprint.Tables[1].ForeignKeys[0].OnDelete = "DROP TABLE"
	if _, err := load(writeBlueprint(t, blueprint)); err == nil {
		t.Fatal("invalid foreign key action was accepted")
	}
}

func TestRemovingModuleFromBlueprintRemovesItsTab(t *testing.T) {
	blueprint := testBlueprint()
	blueprint.Modules = append(blueprint.Modules, Module{
		Name:    "user_management",
		Package: "user_management",
		Master:  Entity{Name: "users_profile"},
		Detail:  Entity{Name: "userdetails"},
	})
	outputs, err := renderBlueprint(blueprint)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(outputs["modules.go"]), `container.NewTabItem("userdetails"`) {
		t.Fatal("configured userdetails module is missing its tab")
	}

	blueprint.Modules = blueprint.Modules[:1]
	outputs, err = renderBlueprint(blueprint)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(outputs["modules.go"]), "userdetails") || strings.Contains(string(outputs["modules.go"]), "generated/user_management") {
		t.Fatalf("removed module remains in generated tabs: %s", outputs["modules.go"])
	}
	if _, exists := outputs[filepath.Join("user_management", "user_management_view.go")]; exists {
		t.Fatal("removed module's source was still generated")
	}
}

func TestGenerateDoesNotWriteWhenAnyModuleFailsToRender(t *testing.T) {
	blueprint := testBlueprint()
	invalid := blueprint.Modules[0]
	invalid.Name = "other"
	invalid.Package = "not valid"
	invalid.Master.Name = "other_master"
	invalid.Detail.Name = "other_detail"
	blueprint.Modules = append(blueprint.Modules, invalid)

	root := t.TempDir()
	destination := filepath.Join(root, "voting", "voting.sql")
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := generateBlueprint(blueprint, root); err == nil {
		t.Fatal("expected invalid generated Go to fail formatting")
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original" {
		t.Fatalf("existing output changed after render failure: %q", content)
	}
}
