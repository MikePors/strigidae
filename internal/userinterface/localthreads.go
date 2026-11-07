package userinterface

import "github.com/MikePors/strigidae/internal/localthreads"

// DefaultLocalThreadLimit is the most local threads kept for one pull request.
// When another is asked, the oldest goes, from memory and from the file.
const DefaultLocalThreadLimit = localthreads.DefaultLimit

// LocalThread is a plugin's question about diff lines and what came of it. It
// lives in the root model, and in a file under the state directory between
// runs of strig. The store and the rules for it are in internal/localthreads;
// this name is what the views use.
type LocalThread = localthreads.Entry

// LocalThreadsMessage gives views every local thread, after any of them
// changed and to each view as it is pushed. A view keeps the ones of its own
// pull request. Threads are copies; the root owns the store.
type LocalThreadsMessage struct{ Threads []LocalThread }
