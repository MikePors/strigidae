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

// application is everything the TUI needs, resolved before it starts.
type application struct {
	client     github.Client
	repository github.Repository
	theme      theme.Theme
	// dashboard is set when strig runs outside a checkout without -R: there is
	// no repository to list, so the first view is the dashboard of config.
	dashboard bool
	// config holds the tabs and sections the dashboard shows.
	config config.Config
}

// newApplication resolves the repository to show and a client for its host.
func newApplication(repositoryFlag string, configuration config.Config, dependencies dependencies) (application, error) {
	target, err := resolveTarget(repositoryFlag, dependencies)
	// Outside a checkout there is no host either: the client uses the gh default.
	dashboard := repositoryFlag == "" && errors.Is(err, gitrepository.ErrNotCheckout)
	if err != nil && !dashboard {
		return application{}, err
	}
	client, err := dependencies.newClient(target.Host)
	if errors.Is(err, github.ErrNotAuthenticated) {
		return application{}, &signInError{host: target.Host, err: err}
	}
	if err != nil {
		return application{}, err
	}
	return application{client: client, repository: target.Repository, theme: theme.Default(), dashboard: dashboard, config: configuration}, nil
}

// resolveTarget decides which repository to show: -R if given, else the
// current checkout's remote. Outside a checkout it fails with
// gitrepository.ErrNotCheckout.
func resolveTarget(repositoryFlag string, dependencies dependencies) (gitrepository.Remote, error) {
	if repositoryFlag != "" {
		repository, err := github.ParseRepository(repositoryFlag)
		if err != nil {
			return gitrepository.Remote{}, &usageError{fmt.Sprintf("-R: %v", err)}
		}
		// No host: the client uses the gh default (GH_HOST or github.com).
		return gitrepository.Remote{Repository: repository}, nil
	}
	directory, err := dependencies.getWorkingDirectory()
	if err != nil {
		return gitrepository.Remote{}, err
	}
	return gitrepository.Infer(context.Background(), dependencies.runner, directory, dependencies.knownHost)
}
