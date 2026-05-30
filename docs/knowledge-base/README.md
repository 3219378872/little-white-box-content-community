# Knowledge Base

Curated, current description of the **esx** (little-white-box) social content
platform for agents and humans. One page per module under `modules/`,
cross-module data flow and main runtime flows under `flows/`, and a total index
in [INDEX.md](INDEX.md).

This knowledge base does not replace `docs/superpowers/specs/` (point-in-time
designs) or `docs/agent-harness/` (task execution records). It is the
context-oriented "how the project works today" layer.

## Page Frontmatter

Every page carries YAML frontmatter:

    ---
    title: user
    tracks:
      - app/user/
    last_synced_commit: <sha>
    last_synced_date: YYYY-MM-DD
    sync_note: ""
    ---

- `title` — page title; module pages match the module name.
- `tracks` — repository paths the page is responsible for covering.
- `last_synced_commit` / `last_synced_date` — last reconciliation point.
- `sync_note` — optional one-line note; used for the lightweight waiver.

## Module Pages

Each module page has fixed sections: 职责, 公开接口与契约, 上游, 下游,
关键文件, 注意事项与陷阱.

Every backend subpackage is covered exactly once: each service under `app/`
that owns Go code and each shared library under `pkg/`.

## Keeping It Current

`python3 scripts/knowledge_base.py check` validates structure and links and is
a blocking CI lint step. Rule K005 is a co-change check: a pull request that
changes a path tracked by a page must also change that page. If the change does
not need a content edit, bump `last_synced_commit` and fill `sync_note` with the
reason — the lightweight waiver.

Adding a new service under `app/` or library under `pkg/` therefore also
requires adding a `modules/` page for it in the same pull request (rule K002).

A weekly `knowledge-base-sync` workflow reports pages whose tracked code has
drifted since `last_synced_commit` so they can be refreshed.
