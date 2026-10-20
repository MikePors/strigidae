// Package github defines the domain types, the Client interface and the typed
// errors that the UI uses to talk to GitHub. It performs no I/O itself: the
// real implementation lives in internal/github/api and an in-memory one in
// internal/github/fake.
package github
