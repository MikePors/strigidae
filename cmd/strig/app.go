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
type application struct {
	client     github.Client
	repository github.Repository
	theme      theme.Theme
	// config holds the tabs and sections the dashboard (#12) will show.
	config config.Config
}

// newApp resolves the repository to show and a client for its host.
func newApplication(repositoryFlag string, configuration config.Config, dependencies dependencies) (application, error) {
	target, err := resolveTarget(repositoryFlag, dependencies)
	if err != nil {
		return application{}, err
	}
	client, err := dependencies.newClient(target.Host)
	if errors.Is(err, github.ErrNotAuthenticated) {
		return application{}, &signInError{host: target.Host, err: err}
	}
	if err != nil {
		return application{}, err
	}
	return application{client: client, repository: target.Repository, theme: theme.Default(), config: configuration}, nil
}

// resolveTarget decides which repository to show: -R if given, else the
// current checkout's remote.
func resolveTarget(repositoryFlag string, dependencies dependencies) (gitrepository.Remote, error) {
	if repositoryFlag != "" {
		repository, err := github.ParseRepository(repositoryFlag)
		if err != nil {
			return gitrepository.Remote{}, &usageError{fmt.Sprintf("-R: %v", err)}
		}
		// No host: the client uses the gh default (GH_HOST or github.com).
		return gitrepository.Remote{Repository: repository}, nil
	}
	directory, err := dependencies.getwd()
	if err != nil {
		return gitrepository.Remote{}, err
	}
	return gitrepository.Infer(context.Background(), dependencies.runner, directory, dependencies.knownHost)
}
