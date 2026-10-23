package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/gitrepository"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// app is everything the TUI needs, resolved before it starts.
type app struct {
	client github.Client
	repo   github.Repository
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
func resolveTarget(repoFlag string, d deps) (gitrepository.Remote, error) {
	if repoFlag != "" {
		repo, err := github.ParseRepository(repoFlag)
		if err != nil {
			return gitrepository.Remote{}, &usageError{fmt.Sprintf("-R: %v", err)}
		}
		// No host: the client uses the gh default (GH_HOST or github.com).
		return gitrepository.Remote{Repo: repo}, nil
	}
	dir, err := d.getwd()
	if err != nil {
		return gitrepository.Remote{}, err
	}
	return gitrepository.Infer(context.Background(), d.runner, dir, d.knownHost)
}
