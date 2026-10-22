package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/gitrepo"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// app is everything the TUI needs, resolved before it starts.
type app struct {
	client github.Client
	repo   github.Repo
	theme  theme.Theme
	// config holds the tabs and sections the dashboard (#12) will show.
	config config.Config
}

// newApp resolves the repository to show and a client for its host.
func newApp(repoFlag string, cfg config.Config, d deps) (app, error) {
	target, err := resolveTarget(repoFlag, d)
	if err != nil {
		return app{}, err
	}
	client, err := d.newClient(target.Host)
	if errors.Is(err, github.ErrNotAuthenticated) {
		return app{}, &signInError{host: target.Host, err: err}
	}
	if err != nil {
		return app{}, err
	}
	return app{client: client, repo: target.Repo, theme: theme.Default(), config: cfg}, nil
}

// resolveTarget decides which repository to show: -R if given, else the
// current checkout's remote.
func resolveTarget(repoFlag string, d deps) (gitrepo.Remote, error) {
	if repoFlag != "" {
		repo, err := github.ParseRepo(repoFlag)
		if err != nil {
			return gitrepo.Remote{}, &usageError{fmt.Sprintf("-R: %v", err)}
		}
		// No host: the client uses the gh default (GH_HOST or github.com).
		return gitrepo.Remote{Repo: repo}, nil
	}
	dir, err := d.getwd()
	if err != nil {
		return gitrepo.Remote{}, err
	}
	return gitrepo.Infer(context.Background(), d.runner, dir, d.knownHost)
}
