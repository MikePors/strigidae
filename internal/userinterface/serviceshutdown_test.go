package userinterface

import (
	"context"
	"testing"
	"time"

	"github.com/MikePors/strigidae/internal/localthreads"
	"github.com/MikePors/strigidae/internal/plugins"
)

// shutdownRunner can keep a run alive past Close's wait, independently of Stop.
type shutdownRunner struct {
	started chan struct{}
	stopped chan struct{}
	release chan struct{}
}

func (runner *shutdownRunner) Run(context.Context, string, plugins.Request) ([]plugins.Effect, error) {
	close(runner.started)
	<-runner.release
	return nil, nil
}
func (runner *shutdownRunner) Stop() { close(runner.stopped) }

func TestCloseStopsServicesEvenWhenAFileWriteIsStuck(test *testing.T) {
	runner := &shutdownRunner{stopped: make(chan struct{})}
	model, _ := pluginModel(test, runner, newPullRequestView(pullRequestSeven))
	model.pluginQuitWait = 20 * time.Millisecond
	model.localFiles.Directory = test.TempDir()
	model.localFiles.Changed(localthreads.Key{Host: "github.com", Repository: pullRequestSeven.Repository, Number: 7})
	release := model.localFiles.HoldForTest()
	defer release()
	closed := make(chan struct{})
	go func() { model.Close(); close(closed) }()
	select {
	case <-runner.stopped:
	case <-time.After(time.Second):
		test.Fatal("stuck flush prevented stopping services")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		test.Fatal("Close exceeded its quit wait")
	}
	release()
	<-model.localFiles.Flushed()
}

func TestCloseStopsServicesEvenWhenARunIsStuck(test *testing.T) {
	runner := &shutdownRunner{started: make(chan struct{}), stopped: make(chan struct{}), release: make(chan struct{})}
	model, _ := pluginModel(test, runner, newPullRequestView(pullRequestSeven))
	model.pluginQuitWait = 500 * time.Millisecond
	run := startRun(model, pullRequestSeven)
	ended := make(chan struct{})
	go func() { run(); close(ended) }()
	<-runner.started
	defer func() { close(runner.release); <-ended }()
	closed := make(chan struct{})
	go func() { model.Close(); close(closed) }()
	select {
	case <-runner.stopped:
	case <-time.After(time.Second):
		test.Fatal("stuck run prevented stopping services")
	}
	// The run is stuck, so only the end of the quit wait lets Close return:
	// Stop must have started before that.
	select {
	case <-closed:
		test.Fatal("Stop started only after the quit wait ended")
	default:
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		test.Fatal("Close exceeded its quit wait")
	}
}
