package stack

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	getter "github.com/hashicorp/go-getter"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	generate         = "generate"
	stackCacheDir    = ".terragrunt-stack"
	defaultStackFile = "terragrunt.stack.hcl"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, subCommand string) error {
	if subCommand == "" {
		return errors.New("No command specified")
	}

	switch subCommand {
	case generate:
		{
			return generateStack(ctx, opts)
		}
	}

	return nil
}

func generateStack(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.TerragrungStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
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
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return errors.New(fmt.Errorf("failed to create base directory: %w", err))
	}

	for _, unit := range stackFile.Units {
		destPath := filepath.Join(baseDir, unit.Path)
		dest, err := filepath.Abs(destPath)
		if err != nil {
			return errors.New(fmt.Errorf("failed to get absolute path for destination '%s': %w", dest, err))
		}

		src := unit.Source
		// set absolute path for source if it's not an absolute path or URL
		if !filepath.IsAbs(unit.Source) && !isURL(unit.Source) {
			src = filepath.Join(opts.WorkingDir, unit.Source)
			src, err = filepath.Abs(src)
			if err != nil {
				opts.Logger.Warnf("failed to get absolute path for source '%s': %v", unit.Source, err)
				src = unit.Source
			}
		}
		opts.Logger.Infof("Processing unit: %s (%s) to %s", unit.Name, src, dest)

		client := &getter.Client{
			Src:             src,
			Dst:             dest,
			Mode:            getter.ClientModeAny,
			Dir:             true,
			DisableSymlinks: true,
			Options: []getter.ClientOption{
				getter.WithInsecure(),
				getter.WithContext(ctx),
				getter.WithGetters(map[string]getter.Getter{
					"file": &StacksFileProvider{},
				}),
			},
		}
		if err := client.Get(); err != nil {
			return errors.New(fmt.Errorf("failed to fetch source '%s' to destination '%s': %w", unit.Source, dest, err))
		}
	}

	return nil
}

func isURL(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

type StacksFileProvider struct {
	client *getter.Client
}

// Get implements downloading functionality
func (p *StacksFileProvider) Get(dst string, u *url.URL) error {
	src := u.Path
	fi, err := os.Stat(src)
	if err != nil {
		return errors.New(fmt.Errorf("source path error: %w", err))
	}

	if fi.IsDir() {
		return p.copyDir(src, dst)
	}
	return p.copyFile(src, dst)
}

// GetFile implements single file download
func (p *StacksFileProvider) GetFile(dst string, u *url.URL) error {
	return p.copyFile(u.Path, dst)
}

// ClientMode determines if we're getting a directory or single file
func (p *StacksFileProvider) ClientMode(u *url.URL) (getter.ClientMode, error) {
	fi, err := os.Stat(u.Path)
	if err != nil {
		return getter.ClientModeInvalid, errors.New(err)
	}

	if fi.IsDir() {
		return getter.ClientModeDir, nil
	}
	return getter.ClientModeFile, nil
}

// SetClient sets the client for this provider
func (p *StacksFileProvider) SetClient(c *getter.Client) {
	p.client = c
}

func (p *StacksFileProvider) copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
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

func (p *StacksFileProvider) copyDir(src, dst string) error {
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
