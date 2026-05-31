package bitbucket

import (
	"regexp"
	"testing"
)

var vcsRe = regexp.MustCompile(`^(git|hg|svn|bzr|fossil|mod)$`)

func TestBitbucketRegistry(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		vcsType    string
		importPath string
		repoURL    string
		wantServer string
		wantVCS    string
		wantDisp   string
		wantSubdir string
	}{
		{
			name:       "default branch and git",
			importPath: "vanity.com/my-repo",
			repoURL:    "https://bitbucket.org/user/my-repo",
			wantServer: "bitbucket.org",
			wantVCS:    "git",
			wantDisp:   "https://bitbucket.org/user/my-repo https://bitbucket.org/user/my-repo/src/main{/dir} https://bitbucket.org/user/my-repo/src/main{/dir}/{file}#lines-{line}",
			wantSubdir: "",
		},
		{
			name:       "custom branch and hg",
			branch:     "dev",
			vcsType:    "hg",
			importPath: "vanity.com/my-repo",
			repoURL:    "https://bitbucket.org/user/my-repo",
			wantServer: "bitbucket.org",
			wantVCS:    "hg",
			wantDisp:   "https://bitbucket.org/user/my-repo https://bitbucket.org/user/my-repo/src/dev{/dir} https://bitbucket.org/user/my-repo/src/dev{/dir}/{file}#lines-{line}",
			wantSubdir: "",
		},
		{
			name:       "with subdirectory",
			importPath: "vanity.com/my-repo/pkg/util",
			repoURL:    "https://bitbucket.org/user/my-repo",
			wantServer: "bitbucket.org",
			wantVCS:    "git",
			wantDisp:   "https://bitbucket.org/user/my-repo https://bitbucket.org/user/my-repo/src/main{/dir} https://bitbucket.org/user/my-repo/src/main{/dir}/{file}#lines-{line}",
			wantSubdir: "pkg/util",
		},
		{
			name:       "mismatched repo name",
			importPath: "vanity.com/other-repo/pkg",
			repoURL:    "https://bitbucket.org/user/my-repo",
			wantServer: "bitbucket.org",
			wantVCS:    "git",
			wantDisp:   "https://bitbucket.org/user/my-repo https://bitbucket.org/user/my-repo/src/main{/dir} https://bitbucket.org/user/my-repo/src/main{/dir}/{file}#lines-{line}",
			wantSubdir: "",
		},
		{
			name:       "with subdirectory and .git suffix in repoURL",
			importPath: "vanity.com/my-repo/pkg/util",
			repoURL:    "https://bitbucket.org/user/my-repo.git",
			wantServer: "bitbucket.org",
			wantVCS:    "git",
			wantDisp:   "https://bitbucket.org/user/my-repo.git https://bitbucket.org/user/my-repo.git/src/main{/dir} https://bitbucket.org/user/my-repo.git/src/main{/dir}/{file}#lines-{line}",
			wantSubdir: "pkg/util",
		},
		{
			name:       "with subdirectory and trailing slash in repoURL",
			importPath: "vanity.com/my-repo/pkg/util",
			repoURL:    "https://bitbucket.org/user/my-repo/",
			wantServer: "bitbucket.org",
			wantVCS:    "git",
			wantDisp:   "https://bitbucket.org/user/my-repo/ https://bitbucket.org/user/my-repo//src/main{/dir} https://bitbucket.org/user/my-repo//src/main{/dir}/{file}#lines-{line}",
			wantSubdir: "pkg/util",
		},
		{
			name:       "partial segment mismatch",
			importPath: "vanity.com/my-repo-special/pkg",
			repoURL:    "https://bitbucket.org/user/my-repo",
			wantServer: "bitbucket.org",
			wantVCS:    "git",
			wantDisp:   "https://bitbucket.org/user/my-repo https://bitbucket.org/user/my-repo/src/main{/dir} https://bitbucket.org/user/my-repo/src/main{/dir}/{file}#lines-{line}",
			wantSubdir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Registry{
				Branch:  tt.branch,
				VCSType: tt.vcsType,
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
