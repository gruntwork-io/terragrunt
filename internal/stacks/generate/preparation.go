package generate

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

func (c *Coordinator) registerOutputs(ctx context.Context, destinations []string) error {
	owner, ok := ctx.Value(generationOwnerKey{}).(generationOwner)
	if !ok || owner.coordinator != c {
		return nil
	}

	paths := make([]string, 0, len(destinations))
	for _, dest := range destinations {
		path, err := util.CanonicalResolvedPath(c.fs, dest, c.opts.WorkingDir)
		if err != nil {
			return err
		}

		paths = append(paths, path)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	claimed := make(map[string]bool, len(paths))
	for _, path := range paths {
		if previous := c.outputs[path]; previous != nil {
			return fmt.Errorf("cannot generate into %q: already produced by stack %q", path, previous.path)
		}

		if claimed[path] {
			return fmt.Errorf("stack %q declares multiple components at %q", owner.path, path)
		}

		claimed[path] = true
		for registered := range c.jobs {
			if registered == owner.path {
				continue
			}

			if vfs.Within(c.fs, path, registered) {
				return fmt.Errorf(
					"cannot generate into %q: stack %q was already requested; generate its producer first",
					path,
					registered,
				)
			}
		}
	}

	for _, path := range paths {
		c.outputs[path] = c.jobs[owner.path]
	}

	return nil
}

// pendingProducer requires c.mu to be held. Failed writes remain a barrier to readers.
func (c *Coordinator) pendingProducer(path string) *generationJob {
	for dest, job := range c.outputs {
		if !vfs.Within(c.fs, dest, path) {
			continue
		}

		select {
		case <-job.filesReady:
			if job.filesErr != nil {
				return job
			}
		default:
			return job
		}
	}

	return nil
}

func (c *Coordinator) waitForFiles(ctx context.Context, path string) error {
	path, err := util.CanonicalResolvedPath(c.fs, path, c.opts.WorkingDir)
	if err != nil {
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		c.mu.Lock()

		producer := c.pendingProducer(path)
		if producer == nil {
			c.mu.Unlock()
			return nil
		}

		select {
		case <-producer.filesReady:
			err := producer.filesErr
			c.mu.Unlock()

			return err
		default:
		}

		owner, _ := ctx.Value(generationOwnerKey{}).(generationOwner)
		if owner.coordinator == c {
			if route := c.route(producer.path, owner.path, make(map[string]bool)); route != nil {
				c.mu.Unlock()

				return fmt.Errorf(
					"stack dependency cycle while waiting for generated files: %s",
					strings.Join(append([]string{owner.path}, route...), " -> "),
				)
			}

			c.edges[owner.path] = append(c.edges[owner.path], producer.path)
		}
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-producer.filesReady:
			err = producer.filesErr
		}

		if owner.coordinator == c {
			c.mu.Lock()
			edges := c.edges[owner.path]
			index := slices.Index(edges, producer.path)
			c.edges[owner.path] = slices.Delete(edges, index, index+1)
			c.mu.Unlock()
		}

		if err != nil {
			return err
		}
	}
}

func (c *Coordinator) filesWritten(ctx context.Context, err error) {
	owner, _ := ctx.Value(generationOwnerKey{}).(generationOwner)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.finishFiles(c.jobs[owner.path], err)
}

// finishFiles requires c.mu to be held. Completion also releases readers after a panic or early failure.
func (c *Coordinator) finishFiles(job *generationJob, err error) {
	select {
	case <-job.filesReady:
	default:
		job.filesErr = err
		close(job.filesReady)
	}
}
