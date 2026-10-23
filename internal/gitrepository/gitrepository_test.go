package gitrepository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestParseURL(t *testing.T) {
	repo := func(o, n string) github.Repo { return github.Repo{Owner: o, Name: n} }
	tests := []struct {
		raw     string
		want    Remote
		wantErr bool
	}{
		{"git@github.com:MikePors/strigidae.git", Remote{"github.com", repo("MikePors", "strigidae")}, false},
		{"git@github.com:MikePors/strigidae", Remote{"github.com", repo("MikePors", "strigidae")}, false},
		{"ghe.example.com:team/app.git", Remote{"ghe.example.com", repo("team", "app")}, false},
		{"ssh://git@github.com/o/r.git", Remote{"github.com", repo("o", "r")}, false},
		{"ssh://git@ghe.example.com:2222/o/r", Remote{"ghe.example.com", repo("o", "r")}, false},
		{"ssh://git@ssh.github.com:443/o/r.git", Remote{"github.com", repo("o", "r")}, false},
		{"https://github.com/o/r.git", Remote{"github.com", repo("o", "r")}, false},
		{"https://github.com/o/r/", Remote{"github.com", repo("o", "r")}, false},
		{"https://user:tok@ghe.example.com/o/r.git", Remote{"ghe.example.com", repo("o", "r")}, false},
		{"git://github.com/o/r.git", Remote{"github.com", repo("o", "r")}, false},
		{"  https://github.com/o/r.git\n", Remote{"github.com", repo("o", "r")}, false},
		{"https://GitHub.com/o/r", Remote{"github.com", repo("o", "r")}, false},
		{"https://github.com:443/o/r.git", Remote{"github.com", repo("o", "r")}, false},
		{"git@ssh.github.com:o/r.git", Remote{"github.com", repo("o", "r")}, false},
		{"git@GitHub.com:o/r.git", Remote{"github.com", repo("o", "r")}, false},
		{"", Remote{}, true},
		{"/home/me/repo", Remote{}, true},
		{"../repo", Remote{}, true},
		{"https://github.com/onlyowner", Remote{}, true},
		{"https://github.com/a/b/c", Remote{}, true},
		{"file:///srv/git/r.git", Remote{}, true},
		{"ftp://github.com/o/r", Remote{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseURL(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// fakeGit answers rev-parse and remote get-url from a table.
func fakeGit(toplevel bool, remotes map[string]string) Runner {
	return func(_ context.Context, _ string, args ...string) (string, error) {
		switch {
		case len(args) > 0 && args[0] == "rev-parse":
			if !toplevel {
				return "", errors.New("not a git repository")
			}
			return "/work/repo", nil
		case len(args) == 3 && args[0] == "remote" && args[1] == "get-url":
			if u, ok := remotes[args[2]]; ok {
				return u, nil
			}
			return "", errors.New("no such remote")
		}
		return "", errors.New("unexpected git call")
	}
}

func TestUnknownHostErrorQuotesEachHost(t *testing.T) {
	err := &UnknownHostError{Hosts: []string{"gitlab.com", "github-work"}}
	want := `remote host "gitlab.com", "github-work" is not a known GitHub host`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestInferKnownHosts(t *testing.T) {
	known := func(h string) bool { return h == "github.com" || h == "ghe.example.com" }
	tests := []struct {
		name      string
		remotes   map[string]string
		want      Remote
		wantHosts []string // set when an UnknownHostError is expected
	}{
		{"known origin", map[string]string{"origin": "git@github.com:o/r.git"}, Remote{"github.com", github.Repo{Owner: "o", Name: "r"}}, nil},
		{
			"gitlab upstream falls back to github origin",
			map[string]string{"upstream": "git@gitlab.com:o/r.git", "origin": "git@github.com:me/r.git"},
			Remote{"github.com", github.Repo{Owner: "me", Name: "r"}},
			nil,
		},
		{"ssh alias only", map[string]string{"origin": "github-work:acme/app.git"}, Remote{}, []string{"github-work"}},
		{"both unknown", map[string]string{"upstream": "git@gitlab.com:o/r.git", "origin": "github-work:acme/app.git"}, Remote{}, []string{"gitlab.com", "github-work"}},
		{"enterprise host", map[string]string{"origin": "https://ghe.example.com/o/r"}, Remote{"ghe.example.com", github.Repo{Owner: "o", Name: "r"}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Infer(context.Background(), fakeGit(true, tt.remotes), "/work", known)
			if tt.wantHosts != nil {
				var ue *UnknownHostError
				if !errors.As(err, &ue) {
					t.Fatalf("err = %v, want *UnknownHostError", err)
				}
				if diff := cmp.Diff(tt.wantHosts, ue.Hosts); diff != "" {
					t.Errorf("hosts (-want +got):\n%s", diff)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestInfer(t *testing.T) {
	up := "git@github.com:upstream-owner/repo.git"
	origin := "https://github.com/fork-owner/repo.git"
	tests := []struct {
		name     string
		toplevel bool
		remotes  map[string]string
		want     github.Repo
		wantErr  error
	}{
		{"origin only", true, map[string]string{"origin": origin}, github.Repo{Owner: "fork-owner", Name: "repo"}, nil},
		{"upstream preferred", true, map[string]string{"origin": origin, "upstream": up}, github.Repo{Owner: "upstream-owner", Name: "repo"}, nil},
		{"unparsable upstream falls back", true, map[string]string{"origin": origin, "upstream": "/local/path"}, github.Repo{Owner: "fork-owner", Name: "repo"}, nil},
		{"no remotes", true, nil, github.Repo{}, ErrNoGitHubRemote},
		{"other remote names ignored", true, map[string]string{"fork": origin}, github.Repo{}, ErrNoGitHubRemote},
		{"outside checkout", false, map[string]string{"origin": origin}, github.Repo{}, ErrNotCheckout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Infer(context.Background(), fakeGit(tt.toplevel, tt.remotes), "/work", nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.Repo != tt.want {
				t.Errorf("repo = %v, want %v", got.Repo, tt.want)
			}
		})
	}
}
