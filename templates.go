package govanityurls

import "html/template"

const styleCSS = `
@import url('https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap');

:root {
    --bg-color: #0b0f19;
    --bg-gradient: radial-gradient(circle at 50% 0%, #1e1b4b 0%, #0b0f19 70%);
    --card-bg: rgba(17, 24, 39, 0.7);
    --card-border: rgba(255, 255, 255, 0.08);
    --card-hover-border: rgba(99, 102, 241, 0.4);
    --text-main: #f3f4f6;
    --text-muted: #9ca3af;
    --primary: #6366f1;
    --primary-hover: #4f46e5;
    --primary-glow: rgba(99, 102, 241, 0.3);
    --success: #10b981;
    --success-bg: rgba(16, 185, 129, 0.1);
    --success-border: rgba(16, 185, 129, 0.2);
    --border: rgba(255, 255, 255, 0.08);
    --radius: 1rem;
    --font-sans: 'Outfit', system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    --font-mono: 'JetBrains Mono', ui-monospace, SFMono-Regular, monospace;
}

body {
    font-family: var(--font-sans);
    background: var(--bg-color);
    background-image: var(--bg-gradient);
    color: var(--text-main);
    line-height: 1.6;
    margin: 0;
    padding: 3rem 1rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    min-height: 100vh;
    box-sizing: border-box;
}

.container {
    width: 100%;
    max-width: 44rem;
    margin: 0 auto;
}

.card {
    background: var(--card-bg);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    border-radius: var(--radius);
    padding: 2.5rem;
    border: 1px solid var(--card-border);
    box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5), 0 10px 10px -5px rgba(0, 0, 0, 0.4);
    transition: border-color 0.3s ease, box-shadow 0.3s ease;
}

.card:hover {
    border-color: var(--card-hover-border);
    box-shadow: 0 25px 30px -5px rgba(0, 0, 0, 0.6), 0 0 20px 0 rgba(99, 102, 241, 0.15);
}

h1, h2 {
    font-weight: 700;
    margin-top: 0;
    letter-spacing: -0.025em;
}

h1 {
    font-size: 2.25rem;
    margin-bottom: 1.5rem;
    text-align: center;
    background: linear-gradient(135deg, #ffffff 30%, #a5b4fc 100%);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
}

h2 {
    font-size: 1.35rem;
    margin-top: 2rem;
    margin-bottom: 1.25rem;
    border-bottom: 1px solid var(--border);
    padding-bottom: 0.75rem;
    color: #e0e7ff;
}

p { color: var(--text-muted); margin-bottom: 1.25rem; }

a {
    color: #a5b4fc;
    text-decoration: none;
    font-weight: 500;
    transition: color 0.2s ease, text-shadow 0.2s ease;
}

a:hover {
    color: #c7d2fe;
    text-shadow: 0 0 8px rgba(199, 210, 254, 0.4);
}

ul, ol { padding-left: 1.5rem; margin: 0 0 1.25rem 0; }
li { padding: 0.4rem 0; color: var(--text-muted); }

.help-list li { border-bottom: none; list-style-position: inside; }

label {
    display: block;
    font-weight: 500;
    margin-bottom: 0.5rem;
    color: #e0e7ff;
    font-size: 0.925rem;
}

.required::after { content: " *"; color: #f87171; }

input[type="text"], select {
    width: 100%;
    padding: 0.75rem 1rem;
    border-radius: 0.5rem;
    background: rgba(17, 24, 39, 0.5);
    border: 1px solid var(--border);
    color: var(--text-main);
    margin-bottom: 1.25rem;
    font-size: 1rem;
    box-sizing: border-box;
    font-family: var(--font-sans);
    transition: border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease;
}

input[type="text"]:focus, select:focus {
    outline: none;
    border-color: var(--primary);
    background: rgba(17, 24, 39, 0.8);
    box-shadow: 0 0 0 3px var(--primary-glow);
}

input:required, select:required {
    border-left: 3px solid var(--primary);
}

button {
    background: linear-gradient(135deg, var(--primary) 0%, #4f46e5 100%);
    color: white;
    font-weight: 600;
    padding: 0.875rem 1.5rem;
    border-radius: 0.5rem;
    border: none;
    cursor: pointer;
    width: 100%;
    font-size: 1rem;
    font-family: var(--font-sans);
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.2), 0 0 12px 0 rgba(99, 102, 241, 0.1);
    transition: transform 0.15s ease, background 0.2s ease, box-shadow 0.2s ease;
}

button:hover {
    background: linear-gradient(135deg, #4f46e5 0%, #4338ca 100%);
    box-shadow: 0 6px 12px -1px rgba(0, 0, 0, 0.3), 0 0 16px 0 rgba(99, 102, 241, 0.2);
}

button:active {
    transform: scale(0.985);
}

.code-block {
    background-color: #030712;
    padding: 1.25rem;
    border-radius: 0.5rem;
    font-family: var(--font-mono);
    font-size: 0.9rem;
    color: #38bdf8;
    overflow-x: auto;
    border: 1px solid rgba(255, 255, 255, 0.05);
    box-shadow: inset 0 2px 4px 0 rgba(0, 0, 0, 0.5);
    margin: 1.5rem 0;
}

code {
    background-color: #111827;
    padding: 0.2rem 0.4rem;
    border-radius: 0.25rem;
    font-family: var(--font-mono);
    font-size: 0.9em;
    color: #f472b6;
    border: 1px solid rgba(255, 255, 255, 0.05);
}

.back-link {
    display: inline-block;
    margin-top: 2rem;
    font-size: 0.9rem;
    text-align: center;
}

.btn-group {
    display: flex;
    gap: 1.25rem;
    justify-content: center;
    margin-top: 2rem;
}

.btn {
    display: inline-block;
    padding: 0.875rem 1.75rem;
    border-radius: 0.5rem;
    text-decoration: none;
    font-weight: 600;
    text-align: center;
    font-size: 0.95rem;
    transition: transform 0.15s ease, box-shadow 0.2s ease, background 0.2s ease;
}

.btn:active {
    transform: scale(0.985);
}

.btn-primary {
    background: linear-gradient(135deg, var(--primary) 0%, #4f46e5 100%);
    color: white;
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.2), 0 0 12px 0 rgba(99, 102, 241, 0.1);
}

.btn-primary:hover {
    background: linear-gradient(135deg, #4f46e5 0%, #4338ca 100%);
    text-decoration: none;
    color: white;
    box-shadow: 0 6px 12px -1px rgba(0, 0, 0, 0.3), 0 0 16px 0 rgba(99, 102, 241, 0.2);
}

.btn-secondary {
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid var(--border);
    color: var(--text-main);
}

.btn-secondary:hover {
    background: rgba(255, 255, 255, 0.1);
    text-decoration: none;
    color: white;
}

.module-list {
    list-style: none;
    padding: 0;
    margin: 1.5rem 0;
}

.module-item {
    background: rgba(255, 255, 255, 0.02);
    border: 1px solid var(--border);
    border-radius: 0.5rem;
    margin-bottom: 0.75rem;
    transition: background-color 0.2s ease, border-color 0.2s ease;
}

.module-item:hover {
    background: rgba(255, 255, 255, 0.05);
    border-color: rgba(99, 102, 241, 0.3);
}

.module-link {
    display: block;
    padding: 0.875rem 1.25rem;
    color: var(--text-main);
    font-family: var(--font-mono);
    font-size: 0.95rem;
    font-weight: 500;
}

.module-link:hover {
    color: #a5b4fc;
    text-shadow: 0 0 8px rgba(199, 210, 254, 0.4);
    text-decoration: none;
}

/* Responsive adjustments */
@media (max-width: 640px) {
    body { padding: 1.5rem 0.5rem; }
    .card { padding: 1.75rem; }
    .btn-group { flex-direction: column; gap: 0.75rem; }
    .btn { width: 100%; box-sizing: border-box; }
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
            <ul class="module-list">
                {{range .Paths}}
                <li class="module-item">
                    <a class="module-link" href="https://pkg.go.dev/{{$.Host}}/{{.}}">{{$.Host}}/{{.}}</a>
                </li>
                {{else}}
                <li style="text-align: center; color: var(--text-muted); padding: 1.5rem 0;">No modules found.</li>
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
            <div style="margin-top: 1.5rem; padding: 1rem; background-color: var(--success-bg); border: 1px solid var(--success-border); border-radius: 0.5rem; color: var(--success); font-weight: 500;">
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
                <li><strong>Bitbucket:</strong> <code>https://bitbucket.org/&lt;user&gt;/&lt;repo&gt; https://bitbucket.org/&lt;user&gt;/&lt;repo&gt;/src/main{/dir} https://bitbucket.org/&lt;user&gt;/&lt;repo&gt;/src/main{/dir}/{file}#lines-{line}</code></li>
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
