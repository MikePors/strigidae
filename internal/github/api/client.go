// Package api implements github.Client on top of go-gh's GraphQL and REST
// clients. It is the only package that talks to the network; GraphQL response
// shapes stay private here and are mapped to the domain types.
package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
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
	gql  *ghapi.GraphQLClient
	rest *ghapi.RESTClient
	// diff is a REST client that asks for the unified diff media type.
	diff  *ghapi.RESTClient
	limit *limitTracker
	now   func() time.Time
	host  string
}

var _ github.Client = (*Client)(nil)

// New builds a Client. It returns an error wrapping github.ErrNotAuthenticated
// when no token can be found.
func New(opts Options) (*Client, error) {
	host := opts.Host
	if host == "" {
		host, _ = auth.DefaultHost()
	}
	token := opts.Token
	if token == "" {
		source := opts.TokenSource
		if source == nil {
			source = func(h string) string { t, _ := auth.TokenForHost(h); return t }
		}
		token = source(host)
	}
	if token == "" {
		return nil, fmt.Errorf("host %s: %w", host, github.ErrNotAuthenticated)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	// Setting Transport (always, to observe rate-limit headers) means go-gh
	// skips its config-based resolution, so Host and Token are given explicitly.
	base := opts.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	tracker := &limitTracker{base: base}
	co := ghapi.ClientOptions{
		Host:         host,
		APIHost:      opts.APIHost,
		AuthToken:    token,
		Transport:    tracker,
		LogIgnoreEnv: true,
	}
	gql, err := ghapi.NewGraphQLClient(co)
	if err != nil {
		return nil, fmt.Errorf("create graphql client: %w", err)
	}
	rest, err := ghapi.NewRESTClient(co)
	if err != nil {
		return nil, fmt.Errorf("create rest client: %w", err)
	}
	co.Headers = map[string]string{"Accept": diffAccept}
	diff, err := ghapi.NewRESTClient(co)
	if err != nil {
		return nil, fmt.Errorf("create diff client: %w", err)
	}
	return &Client{gql: gql, rest: rest, diff: diff, limit: tracker, now: now, host: host}, nil
}

// Host returns the GitHub host this client talks to.
func (c *Client) Host() string { return c.host }

// query runs a GraphQL operation and maps failures to typed errors.
func (c *Client) query(ctx context.Context, query string, vars map[string]any, out any) error {
	if err := c.gql.DoWithContext(ctx, query, vars, out); err != nil {
		return c.mapError(err)
	}
	return nil
}

// limitTracker remembers, per rate-limit bucket (the X-RateLimit-Resource
// header: "core", "graphql", "search", ...), when the service said the limit
// resets. GraphQL rate-limit errors arrive as HTTP 200 whose body does not say
// when, and REST and GraphQL buckets reset at different times.
type limitTracker struct {
	base http.RoundTripper

	mu     sync.Mutex
	resets map[string]time.Time
}

func (t *limitTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	resource := resp.Header.Get("X-RateLimit-Resource")
	if reset, ok := parseReset(resp.Header.Get("X-RateLimit-Reset")); ok && resource != "" {
		t.mu.Lock()
		if t.resets == nil {
			t.resets = map[string]time.Time{}
		}
		t.resets[resource] = reset
		t.mu.Unlock()
	}
	return resp, nil
}

// resetFor returns the last reset time seen for the bucket, or zero.
func (t *limitTracker) resetFor(resource string) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.resets[resource]
}
