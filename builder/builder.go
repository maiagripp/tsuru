// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder

import (
	"archive/tar"
	"bytes"
	"context"
	"io"

	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
)

var DefaultBuilder = "docker"

type BuildOpts struct {
	BuildFromFile       bool
	Rebuild             bool
	Redeploy            bool
	IsTsuruBuilderImage bool
	ArchiveURL          string
	ArchiveFile         io.Reader
	ArchiveTarFile      io.ReadCloser
	ArchiveSize         int64
	ImageID             string
	Tag                 string
	Message             string
}

// Builder is the basic interface of this package.
type Builder interface {
	Build(ctx context.Context, p provision.BuilderDeploy, app provision.App, evt *event.Event, opts *BuildOpts) (appTypes.AppVersion, error)
}

var builders = make(map[string]Builder)

// PlatformBuilder is a builder where administrators can manage
// platforms (automatically adding, removing and updating platforms).
type PlatformBuilder interface {
	PlatformBuild(context.Context, appTypes.PlatformOptions) ([]string, error)
	PlatformRemove(ctx context.Context, name string) error
}

// Register registers a new builder in the Builder registry.
func Register(name string, builder Builder) {
	builders[name] = builder
}

// GetForProvisioner gets the builder required by the provisioner.
func GetForProvisioner(p provision.Provisioner) (Builder, error) {
	builder, err := get(p.GetName())
	if err != nil {
		if _, ok := p.(provision.BuilderDeployDockerClient); ok {
			return get("docker")
		} else if _, ok := p.(provision.BuilderDeployKubeClient); ok {
			return get("kubernetes")
		}
	}
	return builder, err
}

// get gets the named builder from the registry.
func get(name string) (Builder, error) {
	b, ok := builders[name]
	if !ok {
		return nil, errors.Errorf("unknown builder: %q", name)
	}
	return b, nil
}

// Registry returns the list of registered builders.
func Registry() ([]Builder, error) {
	registry := make([]Builder, 0, len(builders))
	for _, b := range builders {
		registry = append(registry, b)
	}
	return registry, nil
}

func PlatformBuild(ctx context.Context, opts appTypes.PlatformOptions) ([]string, error) {
	builders, err := Registry()
	if err != nil {
		return nil, err
	}
	opts.ExtraTags = []string{"latest"}
	multiErr := tsuruErrors.NewMultiError()
	var builtImgs []string
	for _, b := range builders {
		if platformBuilder, ok := b.(PlatformBuilder); ok {
			var imgs []string
			imgs, err := platformBuilder.PlatformBuild(ctx, opts)
			builtImgs = append(builtImgs, imgs...)
			if err == nil {
				return builtImgs, nil
			}
			multiErr.Add(err)
		}
	}
	if multiErr.Len() > 0 {
		return builtImgs, multiErr
	}
	return builtImgs, errors.New("No builder available")
}

func PlatformRemove(ctx context.Context, name string) error {
	builders, err := Registry()
	if err != nil {
		return err
	}
	multiErr := tsuruErrors.NewMultiError()
	for _, b := range builders {
		if platformBuilder, ok := b.(PlatformBuilder); ok {
			err = platformBuilder.PlatformRemove(ctx, name)
			if err == nil {
				return nil
			}
			multiErr.Add(err)
		}
	}
	if multiErr.Len() > 0 {
		return multiErr
	}
	return errors.New("No builder available")
}

func CompressDockerFile(data []byte) io.Reader {
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	writer.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Mode: 0644,
		Size: int64(len(data)),
	})
	writer.Write(data)
	writer.Close()
	return &buf
}
