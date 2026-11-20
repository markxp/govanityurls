---
name: update-go-import-rules
description: >-
  Updates Go vanity URL import (`go-import` and `go-source`) validation rules,
  registry fixers, HTML templates, workspace rule specifications, and
  `README.md` documentation. Use when modifying `go-import` / `go-source` meta
  tag formats, adding or changing supported VCS types (`git`, `hg`, `svn`,
  `bzr`, `fossil`, `mod`), updating `subdir` or `display` placeholder logic, or
  documenting `go-import` rule changes in `README.md`.
---

# Update Go Vanity URL Import Rules & Documentation

Follow this runbook whenever changing `go-import` or `go-source` behavior, validation constraints, VCS providers, or meta tag output in `govanityurls`, and keep `README.md` synchronized with those changes.

---

## 1. Upstream Specification Check

1. Verify the authoritative Go toolchain specification locally before altering rules:
   ```bash
   go help importpath
   ```
2. Cross-check the workspace specification rule in [follow-go-module-import-spec.md](file:///home/markxp/workspace/govanityurls/.agent/rules/follow-go-module-import-spec.md).
3. Enforce core Go tool invariants:
   * `?go-get=1` requests **must** render `<meta name="go-import" ...>` and `<meta name="go-source" ...>` inside `<head>` before any raw CSS or JavaScript.
   * Human/browser requests (without `go-get=1`) **must not** emit `go-import` meta tags.
   * `subdir` (Go 1.25+) requires git/VCS release tags in subdirectories to be prefixed as `<subdir>/vX.Y.Z`.
   * `vcs: mod` points `repo-root` to a Go Module Proxy URL (`https://go.dev/ref/mod#goproxy-protocol`).

---

## 2. Synchronize Codebase Touchpoints

Every change to `go-import` or `go-source` rules must remain consistent across all 5 layers:

| Layer | File | What to Update |
| :--- | :--- | :--- |
| **Workspace Spec Rule** | [.agent/rules/follow-go-module-import-spec.md](file:///home/markxp/workspace/govanityurls/.agent/rules/follow-go-module-import-spec.md) | HTTP handling invariants, `<meta>` syntax, field table (`Path`, `Repo`, `VCS`, `Display`, `Subdir`), and display placeholders (`{/dir}`, `{dir}`, `{file}`, `{line}`). |
| **Storage Schema** | [storage/storage.go](file:///home/markxp/workspace/govanityurls/storage/storage.go) | `RepoConfig` struct tags (`firestore`, `yaml`, `json`) if configuration fields are added or renamed. |
| **Validation & Fixer** | [registry/registry.go](file:///home/markxp/workspace/govanityurls/registry/registry.go) | `Registry` interface comments, `RepoConfigFixer.Fix` regexes (`repoRe`, `vcsRe`), 3-field `Display` validation, and `ExtractSubdir`. |
| **Provider Registries** | [contrib/github/github.go](file:///home/markxp/workspace/govanityurls/contrib/github/github.go), [contrib/bitbucket/bitbucket.go](file:///home/markxp/workspace/govanityurls/contrib/bitbucket/bitbucket.go), [contrib/cloudsourcerepo/cloudsourcerepo.go](file:///home/markxp/workspace/govanityurls/contrib/cloudsourcerepo/cloudsourcerepo.go) | Provider-specific `ServerName()`, `VCS()`, `Display()`, and `Subdir()` implementations. |
| **HTTP Routing & HTML** | [app.go](file:///home/markxp/workspace/govanityurls/app.go), [templates.go](file:///home/markxp/workspace/govanityurls/templates.go) | Longest-prefix module resolution (`getRepo`), `/_admin` form parsing (`postRegisterRepoForm`), `<select name="vcs">` options & help cards (`registerRepoTmpl`), and `<meta>` rendering (`goVanityMetaTagTmpl`). |

---

## 3. Update `README.md` (Mandatory Sync)

Whenever `go-import` or `go-source` rules change, update [README.md](file:///home/markxp/workspace/govanityurls/README.md) in the same change set:

1. **Features (`## Features`)**:
   * Reflect newly supported `go-import` capabilities (e.g., `subdir` module roots, `vcs: mod` proxy protocol, new VCS backends, or `go-source` template rules).
2. **Repository Configuration Schema (`### Repository Configuration Schema`)**:
   * Ensure every `RepoConfig` field (`path`, `repo`, `vcs`, `display`, `subdir`, etc.) is listed with accurate **Required** status (`Yes` / `No`, noting when `Registry` auto-fills defaults for known hosts).
   * Keep the allowed `vcs` enumeration in `README.md` strictly identical to `vcsRe` in [registry/registry.go](file:///home/markxp/workspace/govanityurls/registry/registry.go) (currently `git`, `hg`, `svn`, `bzr`, `fossil`, `mod`) and `<select name="vcs">` in [templates.go](file:///home/markxp/workspace/govanityurls/templates.go).
3. **Examples & Subcommand Docs**:
   * Update configuration examples if schema semantics changed.
   * Check [vanity.yaml](file:///home/markxp/workspace/govanityurls/vanity.yaml), [cmd/run/README.md](file:///home/markxp/workspace/govanityurls/cmd/run/README.md), and [cmd/gae/README.md](file:///home/markxp/workspace/govanityurls/cmd/gae/README.md) if environment variables or registration payloads are affected.

---

## 4. Verification Checklist

1. **Add/Update Unit Tests**:
   * Validation & `Fix()` cases in [registry/registry_test.go](file:///home/markxp/workspace/govanityurls/registry/registry_test.go).
   * Provider `Display()` / `Subdir()` cases in `contrib/*/*_test.go`.
   * End-to-end `?go-get=1` vs browser HTML rendering and `/_admin` registration in [app_test.go](file:///home/markxp/workspace/govanityurls/app_test.go).
2. **Run Test Suite**:
   ```bash
   go test -race ./...
   ```
3. **Audit Drift Across Files**:
   Verify that `vcsRe` in `registry/registry.go`, `<option>` tags in `templates.go`, the validation table in `.agent/rules/follow-go-module-import-spec.md`, and the schema table in `README.md` all match without discrepancy.
