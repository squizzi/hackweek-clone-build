package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/containerd/console"
	git "github.com/go-git/go-git"
	"github.com/go-git/go-git/plumbing/transport/http"
	"github.com/moby/buildkit/client"
	dockerbuild "github.com/moby/buildkit/frontend/dockerfile/builder"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx := context.Background()

	// Get the job details from the loaded env vars when the job was submitted
	pushLocation := os.Args[1]
	gitRepository := os.Args[2]
	gitAuthToken := os.Args[4]
	gitRef := os.Args[3]

	logrus.Infof("Starting local image build of image: %s", pushLocation)
	logrus.Infof("Cloning git repository: %s with authtoken credentials", gitRepository)

	// Clone source
	// XXX: For hackweek, we're just cloning into the local container namespace
	// but we'd most likely want to offer clone targets like s3 for example
	// for large builds and caching, etc.
	// - Cache the clone in a directory associated with pushLocation
	directory := fmt.Sprintf("./buildcache/%s", pushLocation)
	err := os.MkdirAll(directory, os.FileMode(0755))
	if err != nil {
		logrus.Fatalf("Failed to create cache directory for clone: %s", err)
	}
	// - Conduct the clone
	r, err := git.PlainClone(directory, false, &git.CloneOptions{
		Auth: &http.BasicAuth{
			Username: "token",
			Password: gitAuthToken,
		},
		URL:        gitRepository,
		RemoteName: gitRef,
	})
	if err != nil {
		if !errors.Is(err, git.ErrRepositoryAlreadyExists) {
			logrus.Fatalf("Failed to clone git repository: %s: %s", gitRepository, err)
		}
		// The repo is cached, open it instead of cloning
		logrus.Info("Repository already cached, opening...")
		r, err = git.PlainOpen(directory)
		if err != nil {
			logrus.Fatalf("Failed to open cached git repository: %s: %s", gitRepository, err)
		}
	}
	// - Get hash to append to pushed tag
	// TODO (squizzi): Make this an optional flag type system where users
	// can specify if they want commit sha's, branches, etc. in their tag name
	ref, err := r.Head()
	if err != nil {
		logrus.Fatalf("Failed to construct git tag ref for pushed tag result: %s", err)
	}
	gitHash := ref.Hash()

	// Build
	// - Get a buildkit client
	c, err := client.New(ctx, "", client.WithFailFast)
	if err != nil {
		logrus.Fatalf("Failed to initialize buildkit client: %s", err)
	}

	buildCtx := directory
	imageName := fmt.Sprintf("%s-%s", pushLocation, gitHash)
	buildOpt := client.SolveOpt{
		Exports: []client.ExportEntry{
			{
				Type: "image",
				Attrs: map[string]string{
					"name":           imageName,
					"push":           "true",
					"push-by-digest": "false",
				},
			},
		},
		LocalDirs: map[string]string{
			"context":    buildCtx,
			"dockerfile": directory,
		},
		Frontend: "",
		FrontendAttrs: map[string]string{
			"filename": "Dockerfile",
		},
	}

	// - Conduct the build using cloned source
	solveCh := make(chan *client.SolveStatus)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		_, err = c.Build(ctx, buildOpt, "", dockerbuild.Build, solveCh)
		return err
	})
	eg.Go(func() error {
		c, _ := console.ConsoleFromFile(os.Stderr)
		return progressui.DisplaySolveStatus(context.TODO(), "", c, os.Stdout, solveCh)
	})
	if err = eg.Wait(); err != nil {
		logrus.Fatalf("Failed to build and push image: %s with buildkit: %s", imageName, err)
		os.Exit(1)
	}
}
