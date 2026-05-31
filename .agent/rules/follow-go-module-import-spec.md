---
trigger: model_decision
description: Specifications and validation rules for Go vanity URL import tags.
---

# Go Vanity URLs Import Specification

This document defines the rules, behavior, and formatting requirements for serving Go vanity URL import metadata. It covers the general HTTP handling, meta tag output structure (including the `subdir` field introduced in Go 1.25), and repository configuration validation.

## 1. HTTP Server Requirements

- **Vanity URL Matching**: The server matches import paths against registered repository configurations using a recursive search (digging upward from the requested sub-package path to locate the root of the Go module).
- **Request Identification**:
  - **Go Tool Request**: When the Go command requests source code (e.g., via `go get`), it queries the URL with the parameter `?go-get=1`. In this case, the server **MUST** return an HTML page containing `<meta>` tags in the `<head>`.
  - **Human/Browser Request**: If `go-get=1` is *not* present, the server **MUST** serve a human-readable documentation or landing page, and it **MUST NOT** include `go-import` tags.
- **Header Placement**: The `<meta>` tags must appear as early in the HTML document as possible, specifically before any raw JavaScript or CSS, to prevent parsing issues with the Go tool's restricted parser.

## 2. Meta Tag Output Syntax

### The `go-import` Tag
The `go-import` tag declares the source code location of the package. It has two formats depending on the Go version compatibility and VCS configuration:

#### Standard Format
```html
<meta name="go-import" content="import-prefix vcs repo-root">
```

#### Subdirectory Format (Supported in Go 1.25+)
For repositories organized with Go module roots in a subdirectory:
```html
<meta name="go-import" content="import-prefix vcs repo-root subdir">
```
*Note: If `subdir` is set, all VCS tags must be prefixed with `subdir` (e.g., `subdir/v1.2.3`).*

#### Go Module Proxy (preferred over VCS)
```html
<meta name="go-import" content="import-prefix mod proxy-url">
```

### The `go-source` Tag
The `go-source` tag provides links to code browsing UIs (e.g., for `pkg.go.dev`):
```html
<meta name="go-source" content="import-prefix ui-home ui-directory ui-file">
```

---

## 3. Configuration Fields and Validation Rules

All repository configurations stored in the system must conform to the following rules:

| Field | Description / Format | Validation Constraints |
| :--- | :--- | :--- |
| **Path** | The URL path prefix representing the module. | **Must start with `/`** (e.g., `/foo`). If registration input is missing the prefix, it must be normalized. |
| **Repo** | The source code repository root URL. | Must use `https://` (e.g., `https://github.com/user/project`). |
| **VCS** | The Version Control System type. | Must be one of: `git`, `hg`, `svn`, `bzr`, `fossil`, or `mod`. |
| **Display** | String of 3 space-separated URLs (`ui-home`, `ui-directory`, `ui-file`). | Must contain at least 3 URL templates. Placeholders: `{/dir}`, `{dir}`, `{file}`, and `{line}`. |
| **Subdir** | Optional directory path inside the VCS repository root. | Allowed for Go 1.25+ module structures. |

### Display Placeholders Reference:
- `{/dir}`: Replaces the directory with `dir`. If `dir` is not empty, it adds a leading slash (e.g., `/tools`, `""`).
- `{dir}`: Replaces the directory with `dir` exactly (e.g., `tools`, `""`).
- `{file}`: Replaces the file with `file` (e.g., `doc.go`).
- `{line}`: Replaces the line with `line` (e.g., `42`).
