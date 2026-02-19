package cloudsourcerepo

import (
	"regexp"
	"testing"
)

// vcs of cloudsource repo is always git. "mod" is for Go 1.25+ module mode.
var vcsRe = regexp.MustCompile(`^(git|mod)$`)

func TestRegistry(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
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
			repoURL:    "https://source.developers.google.com/p/project/r/my-repo",
			wantServer: "source.developers.google.com",
			wantVCS:    "git",
			wantDisp:   "https://source.cloud.google.com/project/my-repo https://source.cloud.google.com/project/my-repo/+/main:{dir} https://source.cloud.google.com/project/my-repo/+/main:{dir}/{file}#L{line}",
			wantSubdir: "",
		},
		{
			name:       "custom branch",
			branch:     "dev",
			importPath: "vanity.com/my-repo",
			repoURL:    "https://source.developers.google.com/p/project/r/my-repo",
			wantServer: "source.developers.google.com",
			wantVCS:    "git",
			wantDisp:   "https://source.cloud.google.com/project/my-repo https://source.cloud.google.com/project/my-repo/+/dev:{dir} https://source.cloud.google.com/project/my-repo/+/dev:{dir}/{file}#L{line}",
			wantSubdir: "",
		},
		{
			name:       "with subdirectory",
			importPath: "vanity.com/my-repo/pkg/util",
			repoURL:    "https://source.developers.google.com/p/project/r/my-repo",
			wantServer: "source.developers.google.com",
			wantVCS:    "git",
			wantDisp:   "https://source.cloud.google.com/project/my-repo https://source.cloud.google.com/project/my-repo/+/main:{dir} https://source.cloud.google.com/project/my-repo/+/main:{dir}/{file}#L{line}",
			wantSubdir: "pkg/util",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Registry{Branch: tt.branch}

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
