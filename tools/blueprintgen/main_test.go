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
			Master: Entity{
				Name:   "voting_event",
				Unique: []string{"title"},
				Fields: []Field{
					{Name: "title", Label: "Title", Type: "text", Required: true},
					{Name: "description", Label: "Description", Type: "text"},
					{Name: "opens_at", Label: "Opens", Type: "datetime", Required: true},
					{Name: "closes_at", Label: "Closes", Type: "datetime", Required: true},
				},
			},
			Detail: Entity{
				Name:   "vote",
				Fields: []Field{{Name: "voter_user_id", Type: "int64", Required: true}},
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
		"container.NewBorder(actions, nil, nil, nil, container.NewVScroll(form))",
	} {
		if !strings.Contains(view, field) {
			t.Errorf("generated view is missing %q", field)
		}
	}
	registry := string(outputs["modules.go"])
	if !strings.Contains(registry, `container.NewTabItem("vote"`) || !strings.Contains(registry, "NewVotingEventView()") {
		t.Fatalf("generated module tabs do not wire the vote view: %s", registry)
	}
	sql := string(outputs[filepath.Join("voting", "voting.sql")])
	if !strings.Contains(sql, "UNIQUE (title)") {
		t.Fatalf("generated master table is missing its unique constraint: %s", sql)
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
