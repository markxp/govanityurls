package github

import (
	"regexp"
	"testing"
)

var vcsRe = regexp.MustCompile(`^(git|hg|svn|bzr|fossil|mod)$`)

func TestGithubRegistry(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		defaultVCS string
		importPath string
		repoURL    string
		wantServer string
		wantVCS    string
		wantDisp   string
		wantSubdir string
	}{
		{
			name:       "default branch",
			importPath: "vanity.com/my-repo",
			repoURL:    "https://github.com/user/my-repo",
			wantServer: "github.com",
			wantVCS:    "git",
			wantDisp:   "https://github.com/user/my-repo https://github.com/user/my-repo/tree/main{/dir} https://github.com/user/my-repo/blob/main{/dir}/{file}#L{line}",
			wantSubdir: "",
		},
		{
			name:       "hg vcs",
			defaultVCS: "hg",
			importPath: "vanity.com/my-repo",
			repoURL:    "https://github.com/user/my-repo",
			wantServer: "github.com",
			wantVCS:    "hg",
			wantDisp:   "https://github.com/user/my-repo https://github.com/user/my-repo/tree/main{/dir} https://github.com/user/my-repo/blob/main{/dir}/{file}#L{line}",
			wantSubdir: "",
		},
		{
			name:       "custom branch",
			branch:     "dev",
			importPath: "vanity.com/my-repo",
			repoURL:    "https://github.com/user/my-repo",
			wantServer: "github.com",
			wantVCS:    "git",
			wantDisp:   "https://github.com/user/my-repo https://github.com/user/my-repo/tree/dev{/dir} https://github.com/user/my-repo/blob/dev{/dir}/{file}#L{line}",
			wantSubdir: "",
		},
		{
			name:       "with subdirectory",
			importPath: "vanity.com/my-repo/pkg/util",
			repoURL:    "https://github.com/user/my-repo",
			wantServer: "github.com",
			wantVCS:    "git",
			wantDisp:   "https://github.com/user/my-repo https://github.com/user/my-repo/tree/main{/dir} https://github.com/user/my-repo/blob/main{/dir}/{file}#L{line}",
			wantSubdir: "pkg/util",
		},
		{
			name:       "mismatched repo name",
			importPath: "vanity.com/other-repo/pkg",
			repoURL:    "https://github.com/user/my-repo",
			wantServer: "github.com",
			wantVCS:    "git",
			wantDisp:   "https://github.com/user/my-repo https://github.com/user/my-repo/tree/main{/dir} https://github.com/user/my-repo/blob/main{/dir}/{file}#L{line}",
			wantSubdir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &GithubRegistry{
				Branch:     tt.branch,
				DefaultVCS: tt.defaultVCS,
			}

			if got := r.ServerName(); got != tt.wantServer {
				t.Errorf("ServerName() = %q, want %q", got, tt.wantServer)
			}

			if got := r.VCS(); got != tt.wantVCS {
				t.Errorf("VCS() = %q, want %q", got, tt.wantVCS)
			}
			if !vcsRe.MatchString(r.VCS()) {
				t.Errorf("VCS() = %q is not a valid VCS value", r.VCS())
			}

			if got := r.Display(tt.importPath, tt.repoURL); got != tt.wantDisp {
				t.Errorf("Display() = %q, want %q", got, tt.wantDisp)
			}

			if got := r.Subdir(tt.importPath, tt.repoURL); got != tt.wantSubdir {
				t.Errorf("Subdir() = %q, want %q", got, tt.wantSubdir)
			}
		})
	}
}
