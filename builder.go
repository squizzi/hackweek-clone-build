package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git"
	"github.com/go-git/go-git/plumbing/transport/http"
	"github.com/moby/buildkit/client"
	dockerbuild "github.com/moby/buildkit/frontend/dockerfile/builder"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()

	// Get the job details from the loaded env vars when the job was submitted
	pushLocation := os.Args[0]
	gitRepository := os.Args[1]
	gitAuthToken := os.Args[3]
	gitRef := os.Args[2]

	logrus.Infof("Starting local image build of image: %s", pushLocation)
	logrus.Infof("Cloning git repository: %s with authtoken credentials", gitRepository)

	// Clone source
	// XXX: For hackweek, we're just cloning into the local container namespace
	// but we'd most likely want to offer clone targets like s3 for example
	// for large builds and caching, etc.
	// - Cache the clone in a directory associated with pushLocation
	directory := fmt.Sprintf("/builder/%s", pushLocation)
	err := os.MkdirAll(directory, os.FileMode(0755))
	if err != nil {
		logrus.Fatalf("Failed to create cache directory for clone: %w", err)
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
		logrus.Fatalf("Failed to clone git repository: %s: %w", gitRepository, err)
	}
	// - Get hash to append to pushed tag
	// TODO (squizzi): Make this an optional flag type system where users
	// can specify if they want commit sha's, branches, etc. in their tag name
	ref, err := r.Head()
	if err != nil {
		logrus.Fatalf("Failed to construct git tag ref for pushed tag result: %w", err)
	}
	gitHash := ref.Hash()

	// Build
	// - Get a buildkit client
	c, err := client.New(ctx, "", client.WithFailFast)
	if err != nil {
		logrus.Fatalf("Failed to initialize buildkit client: %w", err)
	}

	buildCtx := "."
	imageName := fmt.Sprintf("%s-%s", pushLocation, gitHash)
	dockerfile := filepath.Join(buildCtx, "Dockerfile")
	buildOpt := client.SolveOpt{
		Exports: []client.ExportEntry{
			{
				Type: "image",
				Attrs: map[string]string{
					"name": imageName,
					"push": "true",
				},
			},
		},
		LocalDirs: map[string]string{
			"context":    buildCtx,
			"dockerfile": dockerfile,
		},
		Frontend: "dockerfile.v0",
		FrontendAttrs: map[string]string{
			"filename": "Dockerfile",
		},
	}

	// - Conduct the build using cloned source
	solveCh := make(chan *client.SolveStatus)
	_, err = c.Build(ctx, buildOpt, "", dockerbuild.Build, solveCh)
	if err != nil {
		logrus.Fatalf("Failed to build and push image: %s with buildkit: %w", imageName, err)
	}
}
