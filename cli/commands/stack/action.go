package stack

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/experiment"

	"github.com/gruntwork-io/terragrunt/config"
	getter "github.com/hashicorp/go-getter"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	stackCacheDir    = ".terragrunt-stack"
	defaultStackFile = "terragrunt.stack.hcl"
	dirPerm          = 0755
)

// RunGenerate runs the stack command.
func RunGenerate(ctx context.Context, opts *options.TerragruntOptions) error {
	stacksEnabled := opts.Experiments[experiment.Stacks]
	if !stacksEnabled.Enabled {
		return errors.New("stacks experiment is not enabled use --experiment stacks to enable it")
	}

	return generateStack(ctx, opts)
}

func generateStack(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
	stackFile, err := config.ReadStackConfigFile(ctx, opts)

	if err != nil {
		return errors.New(err)
	}

	if err := processStackFile(ctx, opts, stackFile); err != nil {
		return errors.New(err)
	}

	return nil
}
func processStackFile(ctx context.Context, opts *options.TerragruntOptions, stackFile *config.StackConfigFile) error {
	baseDir := filepath.Join(opts.WorkingDir, stackCacheDir)
	if err := os.MkdirAll(baseDir, dirPerm); err != nil {
		return errors.New(fmt.Errorf("failed to create base directory: %w", err))
	}

	for _, unit := range stackFile.Units {
		destPath := filepath.Join(baseDir, unit.Path)
		dest, err := filepath.Abs(destPath)

		if err != nil {
			return errors.New(fmt.Errorf("failed to get absolute path for destination '%s': %w", dest, err))
		}

		client := &getter.Client{
			Dst:             dest,
			Mode:            getter.ClientModeAny,
			Dir:             true,
			DisableSymlinks: true,
			Options: []getter.ClientOption{
				getter.WithContext(ctx),
			},
		}

		// setting custom getters
		client.Getters = map[string]getter.Getter{}

		for getterName, getterValue := range getter.Getters {
			// setting custom getter for file to not use symlinks
			if getterName == "file" {
				client.Getters[getterName] = &stacksFileProvider{}
			} else {
				client.Getters[getterName] = getterValue
			}
		}

		// fetching unit source
		src := unit.Source

		// set absolute path for source if it's not an absolute path or URL
		if !filepath.IsAbs(unit.Source) && !isURL(client, unit.Source) {
			src = filepath.Join(opts.WorkingDir, unit.Source)
			src, err = filepath.Abs(src)

			if err != nil {
				opts.Logger.Warnf("failed to get absolute path for source '%s': %v", unit.Source, err)
				src = unit.Source
			}
		}

		opts.Logger.Debugf("Processing unit: %s (%s) to %s", unit.Name, src, dest)

		client.Src = src

		if err := client.Get(); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

func isURL(client *getter.Client, str string) bool {
	value, err := getter.Detect(str, client.Dst, getter.Detectors)
	if err != nil {
		return false
	}
	// check if starts with file://
	if strings.HasPrefix(value, "file://") {
		return false
	}

	return true
}

// stacksFileProvider is a custom getter for file:// protocol.
type stacksFileProvider struct {
	client *getter.Client
}

// Get implements downloading functionality.
func (p *stacksFileProvider) Get(dst string, u *url.URL) error {
	src := u.Path
	file, err := os.Stat(src)

	if err != nil {
		return errors.New(fmt.Errorf("source path error: %w", err))
	}

	if file.IsDir() {
		return p.copyDir(src, dst)
	}

	return p.copyFile(src, dst)
}

// GetFile implements single file download.
func (p *stacksFileProvider) GetFile(dst string, u *url.URL) error {
	return p.copyFile(u.Path, dst)
}

// ClientMode determines if we're getting a directory or single file.
func (p *stacksFileProvider) ClientMode(u *url.URL) (getter.ClientMode, error) {
	fi, err := os.Stat(u.Path)
	if err != nil {
		return getter.ClientModeInvalid, errors.New(err)
	}

	if fi.IsDir() {
		return getter.ClientModeDir, nil
	}

	return getter.ClientModeFile, nil
}

// SetClient sets the client for this provider.
func (p *stacksFileProvider) SetClient(c *getter.Client) {
	p.client = c
}

func (p *stacksFileProvider) copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return errors.New(err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return errors.New(err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return errors.New(err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return errors.New(err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return errors.New(err)
	}

	return nil
}

func (p *stacksFileProvider) copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return errors.New(err)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return errors.New(err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return errors.New(err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := p.copyDir(srcPath, dstPath); err != nil {
				return errors.New(err)
			}

			continue
		}

		if err := p.copyFile(srcPath, dstPath); err != nil {
			return errors.New(err)
		}
	}

	return nil
}
