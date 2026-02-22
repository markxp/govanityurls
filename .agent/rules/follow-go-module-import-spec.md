---
trigger: model_decision
description: This is our goal.
---

# Goal of the project

This is a library and binary to create a HTTP server, serving Go modules' vanity URLs. 
We register a path to a Go module. If a user asks for a package in the module, we direct him to the documentation page. If a `go get` tool asks for downloading a package, we should return how to access to the zip bundle through Go Proxy Protocol, or how to download the source code and locate its `go.mod` file.

The specification are listed in the next section.

# Source

## Remote import paths

Certain import paths also describe how to obtain the source code for the package using a revision control system.
For code hosted on other servers, import paths may either be qualified with the version control type, or the go tool can dynamically fetch the import path over https/http and discover where the code resides from a <meta> tag in the HTML.

To declare the code location, an import path of the form
```
repository.vcs/path
```

specifies the given repository, with or without the .vcs suffix, using the named version control system, and then the path inside that repository. The supported version control systems are:
```
Bazaar      .bzr
Fossil      .fossil
Git         .git
Mercurial   .hg
Subversion  .svn
```
For example,

```
import "example.org/user/foo.hg"
```
denotes the root directory of the Mercurial repository at example.org/user/foo, and

```
import "example.org/repo.git/foo/bar"
```
denotes the foo/bar directory of the Git repository at example.org/repo.


If the import path is not a known code hosting site and also lacks a version control qualifier, the go tool attempts to fetch the import over https/http and looks for a <meta> tag in the document's HTML <head>.

The meta tag has the form:
```
<meta name="go-import" content="import-prefix vcs repo-root">
```
Starting in Go 1.25, an optional subdirectory will be recognized by the go command:
```
<meta name="go-import" content="import-prefix vcs repo-root subdir">
```
The import-prefix is the import path corresponding to the repository root. It must be a prefix or an exact match of the package being fetched with "go get". If it's not an exact match, another http request is made at the prefix to verify the <meta> tags match.

The meta tag should appear as early in the file as possible. In particular, it should appear before any raw JavaScript or CSS, to avoid confusing the go command's restricted parser.

The vcs is one of "bzr", "fossil", "git", "hg", "svn".

The repo-root is the root of the version control system containing a scheme and not containing a .vcs qualifier.

The subdir specifies the directory within the repo-root where the Go module's root (including its go.mod file) is located. It allows you to organize your repository with the Go module code in a subdirectory rather than directly at the repository's root. If set, all vcs tags must be prefixed with "subdir". i.e. "subdir/v1.2.3"

For example,

```
import "example.org/pkg/foo"
```
will result in the following requests:

```
https://example.org/pkg/foo?go-get=1 (preferred)
http://example.org/pkg/foo?go-get=1  (fallback, only with use of correctly set GOINSECURE) (we do not support http, ignore this.)
```

If that page contains the meta tag
```
<meta name="go-import" content="example.org git https://code.org/r/p/exproj">
```
the go tool will verify that https://example.org/?go-get=1 contains the same meta tag and then download the code from the Git repository at https://code.org/r/p/exproj

If that page contains the meta tag
```
<meta name="go-import" content="example.org git https://code.org/r/p/exproj foo/subdir">
```
the go tool will verify that https://example.org/?go-get=1 contains the same meta tag and then download the code from the "foo/subdir" subdirectory within the Git repository at https://code.org/r/p/exproj

When using modules, an additional variant of the go-import meta tag is recognized and is preferred over those listing version control systems. That variant uses "mod" as the vcs in the content value, as in:

```
<meta name="go-import" content="example.org mod https://code.org/moduleproxy">
```
This tag means to fetch modules with paths beginning with example.org from the module proxy available at the URL https://code.org/moduleproxy.