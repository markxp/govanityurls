package registry

import (
	"regexp"
	"testing"

	"github.com/markxp/govanityurls/storage"
)

func TestRepoConfigFixer_VCSValidation(t *testing.T) {
	vcsRe := regexp.MustCompile(`^(git|hg|svn|bzr|fossil|mod)$`)

	tests := []struct {
		name    string
		vcs     string
		wantErr bool
	}{
		{"valid git", "git", false},
		{"valid hg", "hg", false},
		{"valid svn", "svn", false},
		{"valid bzr", "bzr", false},
		{"valid fossil", "fossil", false},
		{"valid mod", "mod", false},
		{"invalid vcs", "invalid", true},
		{"empty vcs", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixer := RepoConfigFixer{}
			config := &storage.RepoConfig{
				Repo:    "https://example.com/repo",
				VCS:     tt.vcs,
				Display: "home dir file",
			}
			_, err := fixer.Fix("example.com/repo", config)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Fix() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Also verify the regex standalone for clarity
			if tt.vcs != "" && vcsRe.MatchString(tt.vcs) == tt.wantErr {
				t.Errorf("vcsRe.MatchString(%q) = %v, want %v", tt.vcs, !tt.wantErr, !tt.wantErr)
			}
		})
	}
}
