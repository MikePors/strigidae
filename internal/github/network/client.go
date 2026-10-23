// Package network implements github.Client on top of go-gh's GraphQL and REST
// clients. It is the only package that talks to the network; GraphQL response
// shapes stay private here and are mapped to the domain types.
package network

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	githubapi "github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"

	"github.com/MikePors/strigidae/internal/github"
)

const diffAccept = "application/vnd.github.v3.diff"

// Options configures New. The zero value uses the host and credentials that
// the gh CLI would use.
type Options struct {
	// Host is the GitHub host, e.g. "github.com". Empty means go-gh's default
	// (GH_HOST or the gh config), never a hardcoded host.
	Host string
	// Token overrides token discovery.
	Token string
	// TokenSource looks up a token for the host when Token is empty. Empty
	// means go-gh's lookup (GH_TOKEN, GITHUB_TOKEN, gh config, `gh auth token`).
	TokenSource func(host string) string
	// APIHost overrides the API hostname (bare, no scheme); empty means derive
	// it from Host and the gh config.
	APIHost string
	// Transport replaces the HTTP transport; used by tests.
	Transport http.RoundTripper
	// Now returns the current time; used to compute rate-limit resets.
	Now func() time.Time
}

// Client is the real github.Client.
type Client struct {
	graphqlClient *githubapi.GraphQLClient
	rest          *githubapi.RESTClient
	// diff is a REST client that asks for the unified diff media type.
	diff  *githubapi.RESTClient
	limit *limitTracker
	now   func() time.Time
	host  string
}

var _ github.Client = (*Client)(nil)

// New builds a Client. It returns an error wrapping github.ErrNotAuthenticated
// when no token can be found.
func New(options Options) (*Client, error) {
	host := options.Host
	if host == "" {
		host, _ = auth.DefaultHost()
	}
	token := options.Token
	if token == "" {
		source := options.TokenSource
		if source == nil {
			source = func(hostName string) string { token, _ := auth.TokenForHost(hostName); return token }
		}
		token = source(host)
	}
	if token == "" {
		return nil, fmt.Errorf("host %s: %w", host, github.ErrNotAuthenticated)
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	// Setting Transport (always, to observe rate-limit headers) means go-gh
	// skips its config-based resolution, so Host and Token are given explicitly.
	base := options.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	tracker := &limitTracker{base: base}
	clientOptions := githubapi.ClientOptions{
		Host:         host,
		APIHost:      options.APIHost,
		AuthToken:    token,
		Transport:    tracker,
		LogIgnoreEnv: true,
	}
	graphqlClient, err := githubapi.NewGraphQLClient(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("create graphql client: %w", err)
	}
	rest, err := githubapi.NewRESTClient(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("create rest client: %w", err)
	}
	clientOptions.Headers = map[string]string{"Accept": diffAccept}
	diff, err := githubapi.NewRESTClient(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("create diff client: %w", err)
	}
	return &Client{graphqlClient: graphqlClient, rest: rest, diff: diff, limit: tracker, now: now, host: host}, nil
}

// Host returns the GitHub host this client talks to.
func (client *Client) Host() string { return client.host }

// query runs a GraphQL operation and maps failures to typed errors.
func (client *Client) query(ctx context.Context, query string, variables map[string]any, out any) error {
	if err := client.graphqlClient.DoWithContext(ctx, query, variables, out); err != nil {
		return client.mapError(err)
	}
	return nil
}

// limitTracker remembers, per rate-limit bucket (the X-RateLimit-Resource
// header: "core", "graphql", "search", ...), when the service said the limit
// resets. GraphQL rate-limit errors arrive as HTTP 200 whose body does not say
// when, and REST and GraphQL buckets reset at different times.
type limitTracker struct {
	base http.RoundTripper

	mutex  sync.Mutex
	resets map[string]time.Time
}

func (tracker *limitTracker) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := tracker.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	resource := response.Header.Get("X-RateLimit-Resource")
	if reset, ok := parseReset(response.Header.Get("X-RateLimit-Reset")); ok && resource != "" {
		tracker.mutex.Lock()
		if tracker.resets == nil {
			tracker.resets = map[string]time.Time{}
		}
		tracker.resets[resource] = reset
		tracker.mutex.Unlock()
	}
	return response, nil
}

// resetFor returns the last reset time seen for the bucket, or zero.
func (tracker *limitTracker) resetFor(resource string) time.Time {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	return tracker.resets[resource]
}
