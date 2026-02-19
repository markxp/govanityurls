package govanityurls

import "html/template"

const styleCSS = `
:root {
    --bg-color: #f9fafb;
    --card-bg: #ffffff;
    --text-main: #111827;
    --text-muted: #6b7280;
    --primary: #4f46e5;
    --primary-hover: #4338ca;
    --border: #e5e7eb;
    --shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
    --radius: 0.5rem;
    --font-sans: 'Inter', system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
}

body {
    font-family: var(--font-sans);
    background-color: var(--bg-color);
    color: var(--text-main);
    line-height: 1.5;
    margin: 0;
    padding: 2rem 1rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    min-height: 100vh;
}

.container {
    width: 100%;
    max-width: 42rem;
    margin: 0 auto;
}

.card {
    background: var(--card-bg);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    padding: 2rem;
    border: 1px solid var(--border);
}

h1, h2 {
    color: var(--text-main);
    font-weight: 700;
    margin-top: 0;
}

h1 { font-size: 1.875rem; margin-bottom: 1.5rem; text-align: center; }
h2 { font-size: 1.25rem; margin-top: 2rem; margin-bottom: 1rem; border-bottom: 1px solid var(--border); padding-bottom: 0.5rem; }

p { color: var(--text-muted); margin-bottom: 1rem; }

a { color: var(--primary); text-decoration: none; font-weight: 500; transition: color 0.15s; }
a:hover { color: var(--primary-hover); text-decoration: underline; }

ul, ol { padding-left: 1.5rem; margin: 0 0 1rem 0; }
li { padding: 0.5rem 0; color: var(--text-muted); }
ul { list-style-type: disc; }
ol { list-style-type: decimal; }

.help-list li { border-bottom: none; }

label { display: block; font-weight: 500; margin-bottom: 0.25rem; color: var(--text-main); }
.required::after { content: " *"; color: #ef4444; }
input:required, select:required { border-left: 3px solid var(--primary); }

input[type="text"], select {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border-radius: 0.375rem;
    border: 1px solid var(--border);
    margin-bottom: 1rem;
    font-size: 1rem;
    box-sizing: border-box; /* Ensure padding doesn't affect width */
}

input[type="text"]:focus, select:focus {
    outline: 2px solid var(--primary);
    border-color: transparent;
}

button {
    background-color: var(--primary);
    color: white;
    font-weight: 600;
    padding: 0.625rem 1.25rem;
    border-radius: 0.375rem;
    border: none;
    cursor: pointer;
    width: 100%;
    font-size: 1rem;
    transition: background-color 0.15s;
}

button:hover { background-color: var(--primary-hover); }

.code-block {
    background-color: #f3f4f6;
    padding: 1rem;
    border-radius: 0.375rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.875rem;
    color: #1f2937;
    overflow-x: auto;
    border: 1px solid #d1d5db;
    box-shadow: inset 0 2px 4px 0 rgba(0, 0, 0, 0.06);
}

code {
    background-color: #f3f4f6;
    padding: 0.2rem 0.4rem;
    border-radius: 0.25rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.9em;
    color: #df1b1b;
    border: 1px solid #e5e7eb;
}

.back-link {
    display: inline-block;
    margin-top: 1.5rem;
    font-size: 0.875rem;
}

.btn-group { display: flex; gap: 1rem; justify-content: center; margin-top: 2rem; }
.btn {
    display: inline-block;
    padding: 0.625rem 1.25rem;
    border-radius: 0.375rem;
    text-decoration: none;
    font-weight: 500;
    text-align: center;
}
.btn-primary { background: var(--primary); color: white; }
.btn-primary:hover { background: var(--primary-hover); text-decoration: none; }
.btn-secondary { background: white; border: 1px solid var(--border); color: var(--text-main); }
.btn-secondary:hover { background: #f9fafb; text-decoration: none; }

/* Responsive adjustments */
@media (max-width: 640px) {
    .container { padding: 0 1rem; }
    .card { padding: 1.5rem; }
}
`

var indexTmpl = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Custom Import Path for Go Modules - served by {{.Host}}</title>
    <style>` + styleCSS + `</style>
</head>
<body>
    <div class="container">
        <div class="card">
            <h1>{{.Host}}</h1>
            <p style="text-align: center;">Available Go Modules</p>
            <ul>
                {{range .Paths}}
                <li>
                    <a href="https://pkg.go.dev/{{$.Host}}/{{.}}">{{$.Host}}/{{.}}</a>
                </li>
                {{else}}
                <li style="text-align: center; color: var(--text-muted);">No modules found.</li>
                {{end}}
            </ul>
            <div style="text-align: center; margin-top: 2rem; padding-top: 1rem; border-top: 1px solid var(--border);">
                <a href="/_admin" style="font-size: 0.875rem; color: var(--text-muted);">Manage Modules</a>
            </div>
        </div>
    </div>
</body>
</html>
`))

var registerRepoTmpl = template.Must(template.New("registerRepo").Parse(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Register a Go Module</title>
    <style>` + styleCSS + `</style>
</head>
<body>
    <div class="container">
        <div class="card">
            <h1>Register a Go Module</h1>
            <p>Register a repository to our Go vanity URL service.</p>
            
            <form method="POST" action="/_admin">
                <label for="path" class="required">Path</label>
                <input type="text" id="path" name="path" placeholder="e.g. my-repo" value="{{.Path}}" required>
                <p style="font-size: 0.8rem; margin-top: -0.5rem; margin-bottom: 1rem;">This will be served at {{.Host}}/&lt;path&gt;</p>

                <label for="repo" class="required">Code Repository Address</label>
                <input type="text" id="repo" name="repo" placeholder="e.g. https://github.com/user/repo" value="{{.Repo}}" required>

                <label for="vcs">Version Control System</label>
                <select name="vcs" id="vcs">
                    <option value="git" {{if eq .VCS "git"}}selected{{end}}>Git</option>
                    <option value="hg" {{if eq .VCS "hg"}}selected{{end}}>Mercurial</option>
                    <option value="svn" {{if eq .VCS "svn"}}selected{{end}}>Subversion</option>
                    <option value="bzr" {{if eq .VCS "bzr"}}selected{{end}}>Bazaar</option>
                    <option value="fossil" {{if eq .VCS "fossil"}}selected{{end}}>Fossil</option>
                    <option value="mod" {{if eq .VCS "mod"}}selected{{end}}>Go Module Proxy Protocol (Go 1.25+)</option>
                </select>

                <label for="display">Display (Optional)</label>
                <input type="text" id="display" name="display" placeholder="Display string for go-source" value="{{.Display}}">

                <label for="subdir">Subdirectory (Optional)</label>
                <input type="text" id="subdir" name="subdir" placeholder="Path relative to module root within repo" value="{{.Subdir}}">
                <p style="font-size: 0.8rem; margin-top: -0.5rem; margin-bottom: 1rem;">Use if go.mod is not at the repository root.</p>

                <button type="submit">Register/Update Module</button>
            </form>

            {{if .Message}}
            <div style="margin-top: 1.5rem; padding: 1rem; background-color: #ecfdf5; border: 1px solid #a7f3d0; border-radius: 0.375rem; color: #065f46;">
                {{.Message}}
            </div>
            {{end}}

            <div style="text-align: center;">
                <a href="/" class="back-link">&larr; Back to Module List</a>
            </div>
        </div>

        <div class="card" style="margin-top: 2rem;">
            <h2>Help: source code vs. Go module</h2>
            <ol class="help-list">
                <li><strong>simple case:</strong> Repo path matches module path. (1:1 mapping)</li>
                <li><strong>submodule:</strong> If <code>/submod</code> is a nested module, register <code>path="repo/submod"</code>, <code>subdir="submod"</code>.</li>
                <li><strong>subpkg:</strong> If <code>/subpkg</code> is just a package, register the root module only <code>path="repo"</code>.</li>
            </ol>
        </div>

        <div class="card" style="margin-top: 2rem;">
            <h2>Help: go-source Display String</h2>
            <p>The display string determines how <code>pkg.go.dev</code> links to your source code. It follows the format: <code>&lt;home&gt; &lt;directory&gt; &lt;file&gt;</code>.</p>
            <ul class="help-list">
                <li><strong>GitHub:</strong> <code>https://github.com/&lt;user&gt;/&lt;repo&gt; https://github.com/&lt;user&gt;/&lt;repo&gt;/tree/main{/dir} https://github.com/&lt;user&gt;/&lt;repo&gt;/blob/main{/dir}/{file}#L{line}</code></li>
                <li><strong>GitLab:</strong> <code>https://gitlab.com/&lt;user&gt;/&lt;repo&gt; https://gitlab.com/&lt;user&gt;/&lt;repo&gt;/-/tree/main{/dir} https://gitlab.com/&lt;user&gt;/&lt;repo&gt;/-/blob/main{/dir}/{file}#L{line}</code></li>
                <li><strong>Bitbucket:</strong> <code>https://bitbucket.org/&lt;user&gt;/&lt;repo&gt; https://bitbucket.org/&lt;user&gt;/&lt;repo&gt/src/main{/dir} https://bitbucket.org/&lt;user&gt;/&lt;repo&gt/src/main{/dir}/{file}#lines-{line}</code></li>
                <li><strong>Cloud Source Repositories:</strong> <code>https://source.cloud.google.com/p/&lt;project&gt;/r/&lt;repo&gt; https://source.cloud.google.com/p/&lt;project&gt;/r/&lt;repo&gt;/+/main:{dir} https://source.cloud.google.com/p/&lt;project&gt;/r/&lt;repo&gt;/+/main:{dir}/{file}#L{line}</code></li>
            </ul>
        </div>
    </div>
</body>
</html>
`))

var goVanityMetaTagTmpl = template.Must(template.New("vanity").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
{{if .IsGoGet}}
<meta name="go-import" content="{{.Import}} {{.VCS}} {{.Repo}}{{if .Subdir}} {{.Subdir}}{{end}}">
<meta name="go-source" content="{{.Import}} {{.Display}}">
{{else}}
<title>{{.Import}} - Go Module</title>
<style>` + styleCSS + `</style>
{{end}}
</head>
<body>
{{if .IsGoGet}}
    <!-- Metadata only for go get -->
{{else}}
<div class="container">
    <div class="card" style="text-align: center;">
        <h1>{{.Import}}</h1>
        <p>Go vanity URL for the <code>{{.Import}}</code> module.</p>
        
        <div class="code-block" style="text-align: left;">
            $ go get {{.Import}}
        </div>

        <div class="btn-group">
            <a href="https://pkg.go.dev/{{.Import}}/{{.Subpath}}" class="btn btn-primary">Documentation</a>
            <a href="{{.Repo}}" class="btn btn-secondary">Source Code</a>
        </div>
        
        <div style="margin-top: 2rem;">
             <a href="/" class="back-link">View all modules on this server</a>
        </div>
    </div>
</div>
{{end}}
</body>
</html>`))
