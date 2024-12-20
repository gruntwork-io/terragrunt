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
	generate      = "generate"
	stackCacheDir = ".terragrunt-stack"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, subCommand string) error {
	if subCommand == "" {
		return errors.New("No subCommand specified")
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
	//TODO: update stack path
	opts.TerragrungStackConfigPath = filepath.Join(opts.WorkingDir, "terragrunt.stack.hcl")
	stackFile, err := config.ReadStackConfigFile(ctx, opts)
	if err != nil {
		return err
	}

	if err := processStackFile(ctx, opts, stackFile); err != nil {
		return err
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
		src, err = filepath.Abs(src)
		if err != nil {
			opts.Logger.Warnf("failed to get absolute path for source '%s': %v", unit.Source, err)
			src = unit.Source
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
					"file": &CustomFileProvider{},
				}),
			},
		}
		if err := client.Get(); err != nil {
			return fmt.Errorf("failed to fetch source '%s' to destination '%s': %w", unit.Source, dest, err)
		}
	}

	return nil
}

type CustomFileProvider struct {
	client *getter.Client
}

// Get implements downloading functionality
func (p *CustomFileProvider) Get(dst string, u *url.URL) error {
	src := u.Path

	// Check if source exists
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}

	if fi.IsDir() {
		return p.copyDir(src, dst)
	}
	return p.copyFile(src, dst)
}

// GetFile implements single file download
func (p *CustomFileProvider) GetFile(dst string, u *url.URL) error {
	return p.copyFile(u.Path, dst)
}

// ClientMode determines if we're getting a directory or single file
func (p *CustomFileProvider) ClientMode(u *url.URL) (getter.ClientMode, error) {
	fi, err := os.Stat(u.Path)
	if err != nil {
		return getter.ClientModeInvalid, err
	}

	if fi.IsDir() {
		return getter.ClientModeDir, nil
	}
	return getter.ClientModeFile, nil
}

// SetClient sets the client for this provider
func (p *CustomFileProvider) SetClient(c *getter.Client) {
	p.client = c
}

func (p *CustomFileProvider) copyFile(src, dst string) error {
	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %v", err)
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer dstFile.Close()

	// Copy the contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %v", err)
	}

	// Copy file mode
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source file: %v", err)
	}

	return os.Chmod(dst, srcInfo.Mode())
}

func (p *CustomFileProvider) copyDir(src, dst string) error {
	// Create the destination directory
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source directory: %v", err)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to create destination directory: %v", err)
	}

	// Read directory contents
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %v", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := p.copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := p.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
