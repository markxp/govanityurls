# Go Vanity URLs

Go Vanity URLs is a Go library and application framework that serves custom import paths for your Go packages. It parses the requested package path, matches it recursively against registered configurations, and renders the standard HTML `<meta>` tags required by the `go get` command.

---

## Features

- **Go Module Subdirectories (Go 1.25+ feature):** Serves custom `go-import` tags using the new `subdir` field format for repositories where module roots are located in subdirectories.
- **Go Module Proxy Support:** Option to serve package metadata via Go Module Proxy using `vcs: mod`.
- **Google Cloud Source Repositories (CSR) Fallback:** Automatically resolves missing repositories by querying CSR, registering them on the fly.
- **Asynchronous Registration:** Optional background registry worker using Google Cloud Tasks.
- **Modern Observability:** Built-in OpenTelemetry (OTEL) tracing and structured JSON logging compatible with Google Cloud Logging.

---

## Configuration

### Repository Configuration Schema

Repositories are configured dynamically. The schema details are as follows:

| Field | Required | Description |
| :--- | :--- | :--- |
| `path` | **Yes** | The import path prefix (must start with `/`, e.g., `/my-pkg`). |
| `repo` | **Yes** | Root URL of the source repository (e.g., `https://github.com/user/repo`). |
| `vcs` | **Yes** | Version control system (`git`, `hg`, `svn`, `bzr`, or `mod` for Go Module Proxy). |
| `display` | No | Three space-separated URL templates for the `go-source` tag (home, directory, file). |
| `subdir` | No | Optional subdirectory path inside the repository root containing the module (Go 1.25+). |

### Historical Configuration Reference (`vanity.yaml`)

Historically, configurations were defined statically in a `vanity.yaml` file. While the server now supports dynamic registration and storage backends (e.g., Firestore), `vanity.yaml` remains in the codebase for local reference and historical context:

```yaml
host: rakyll.pizza

paths:
  /portmidi:
    repo: https://github.com/rakyll/portmidi
    vcs: git
  /launchpad:
    repo: https://github.com/rakyll/launchpad
    vcs: git
```

---

## Deployment Examples

We provide example implementations of how to deploy and secure Go Vanity URLs:

- **Google App Engine (GAE):** Secures admin interfaces utilizing Identity-Aware Proxy (IAP). See [cmd/gae/README.md](cmd/gae/README.md) for deployment instructions.
- **Google Cloud Run:** Secures admin interfaces utilizing Google OAuth 2.0 / OIDC with email/domain allow lists. See [cmd/run/README.md](cmd/run/README.md) for deployment instructions.

---

## Local Development

Ensure you have **Go 1.26.0+** installed. You can mock Google services using emulators.

### Running Tests

Execute the unit tests:

```bash
$ go test -v ./...
```

If you wish to run integration tests (requiring Firestore/Cloud Tasks mocks or live configurations):

```bash
$ go test -tags=integration -v ./...
```
