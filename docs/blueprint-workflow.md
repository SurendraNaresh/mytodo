# Blueprint-driven modules

## Boundary and recommendation

`blueprint.json` is a design-time contract. It is not an end-user configuration file and it is not executed as SQL at runtime.

The recommended production flow is:

```text
ADMIN reviews JSON -> blueprintgen validates/generates -> review diff -> apply SQL migration -> compile/test -> ship
```

This keeps schema changes auditable and makes failures ordinary Go/SQL failures instead of failures hidden behind a generic runtime interpreter.

The current generic `internal/db.Store` is useful for prototyping and metadata-driven experiments. It should not be the only production model for important reporting or transactional modules. Typed generated tables provide stronger constraints, indexes, foreign keys, query plans, and compile-time names.

## Complexity tradeoffs

### Debugging

A completely data-driven runtime module has several debugging layers: JSON shape, metadata registration, field mapping, validation rule lookup, authorization, SQL construction, and UI layout. A stack trace often ends in generic code rather than the business operation that failed.

The generator moves most of that complexity to build time. Generated files are normal Go and SQL, so a failed operation points at a module and a fixed query. Keep generated files separate from hand-written companion files; regenerate rather than manually editing generated output.

### UI sizing and layout

Do not put pixel sizes in a blueprint. The blueprint should describe fields, order, labels, and visibility. Fyne should allocate space through `container.NewBorder`, `container.NewGridWithColumns`, `container.NewVBox`, `container.NewForm`, and minimum sizes. A generated view should express relationships, not screen coordinates.

Use these rules:

- The window chooses a desktop minimum size only; mobile lets the platform choose the window size.
- Forms should be scrollable when their minimum height can exceed the viewport.
- Master/detail panes should use a border or split container and allow the detail pane to shrink.
- A field renderer owns its minimum size; the module definition does not force a global width.
- Theme is an application concern. A module can request semantic emphasis, but should not inject arbitrary colors or fonts.

The dashboard keeps the existing application under a `mytodo-` tab. The generator writes `generated/modules.go`, which registers one tab per blueprint module using its detail table name as the tab title and its generated master view as content. Adding a module therefore updates the tab registry on regeneration without a hand-maintained dashboard import.

## Security model

Values must always be SQL parameters (`?`). The generated controller does this for all field values.

Identifiers are different: table and column names cannot be SQL parameters. They are accepted only after blueprint validation, converted into generated source, reviewed, and compiled. Never accept table names, column names, or SQL fragments from a user at runtime.

The generator rejects unknown JSON keys and names unless they match `^[a-z][a-z0-9]*(_[a-z0-9]+)*$`; underscores must separate non-empty segments. It allows only the known field types `text`, `int64`, `bool`, and `datetime`. Keep this allow-list strict.

Authorization must be checked in the controller/service boundary, not only in the view:

1. Authenticate the session.
2. Check the role and module action.
3. Check ownership or parent access.
4. Validate the payload.
5. Execute a parameterized transaction.
6. Return only authorized rows.

ADMIN authorization to alter the blueprint is not enough by itself. A production deployment should review the generated diff and apply database changes through a controlled migration process. The optional Builder UI should later edit a versioned blueprint and submit a change for review; it should not execute arbitrary DDL directly.

## Add a module: voting event and vote

The checked-in `blueprint.json` contains this example:

- Master: `voting_event`
- Detail: `vote`
- Parent key: `voting_event_id`
- Unique rule: one vote per `voter_user_id` for each `voting_event`

Run the generator from the repository root. An administrator can also select
**Generate UI** in the application sidebar; this runs the same generator after
confirmation. It writes Go/SQL source only, so the running application does not
hot-load the generated tabs. The app must be rebuilt and restarted afterward.

### PowerShell

```powershell
go run ./tools/blueprintgen -blueprint .\blueprint.json -out .\generated
git diff -- .\generated
go test ./...
go build -o .\mytodo.exe .
.\mytodo.exe
```

### Bash

```bash
go run ./tools/blueprintgen -blueprint ./blueprint.json -out ./generated
git diff -- ./generated
go test ./...
go build -o ./mytodo .
./mytodo
```

Inspect the result before applying it:

```text
generated/voting/voting.sql
 generated/voting/voting_model.go
 generated/voting/voting_controller.go
 generated/voting/voting_view.go
```

Apply `voting.sql` through a versioned migration. Do not make the application run arbitrary generated DDL on startup. Add domain rules, authorization, and the real Fyne master-detail screen in hand-written companion files beside the generated files.

### `user_management` module

The checked-in module uses lowercase field identifiers and references only
defined fields. Its master `unique` list is emitted as one composite
constraint, so `(username, email)` must be unique as a pair; it does not make
each column independently unique. The detail has no unique constraint because
no phone-number uniqueness rule was specified. Generated generic CRUD does not
hash credentials, so password storage must remain in the existing auth-aware
user model rather than being added as a plain generated field.

## Blueprint field rules

Each module has `name`, `package`, `master`, and `detail`. Each entity has a table `name`, a display `label`, and `fields`:

```json
{
  "name": "choice",
  "label": "Choice",
  "type": "text",
  "required": true
}
```

Supported field types are intentionally small. Add a type only when the generator defines its SQL type, Go type, input widget, validation, and migration behavior together.

The optional top-level `types` array is also decoded and retained. Each entry supports `type_key`, `type_id`, nullable `parent_type_id`, a string-valued `header` map, `logic_rule.validation`, and `ui.list_cols` / `ui.form_order`. Unknown struct fields are rejected so misspelled configuration is not silently discarded; keys inside `header` are intentionally dynamic.

Generated controllers provide create, get, list, update, and delete functions for both the master and detail entities. Detail lists are scoped by the master ID. These are data-access primitives, not a substitute for authorization, payload validation, or optimistic concurrency checks in the service boundary. The generator renders and formats every module before staging and replacing any output files.

## Testing checklist

For every generated module:

- malformed JSON is rejected;
- unknown keys, invalid identifiers, cross-module ID collisions, and unsupported types are rejected;
- a generation/rendering failure leaves existing generated files unchanged;
- generated SQL is reviewed and applied to a disposable database;
- required fields and unique constraints are tested;
- parent access is tested for an authorized and unauthorized user;
- optimistic concurrency behavior is tested before exposing update/delete;
- desktop and mobile layouts are exercised with narrow and wide windows;
- generated output is regenerated in CI and checked for a clean diff.
