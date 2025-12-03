package helpers

import (
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/mattn/go-shellwords"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
)

func ContExecNoOutput(tb testing.TB, container *testcontainers.DockerContainer, cmd string, options ...tcexec.ProcessOption) {
	tb.Helper()
	ctx := tb.Context()

	args, err := shellwords.Parse(cmd)
	require.NoError(tb, err)

	c, output, err := container.Exec(ctx, args, options...)
	require.NoError(tb, err)

	outbytes, _ := io.ReadAll(output)
	require.Zero(tb, c, string(outbytes))
}

type tLogger struct {
	tb testing.TB
}

func (l tLogger) Printf(format string, v ...any) {
	l.tb.Helper()
	l.tb.Logf(format, v...)
}

func RunContainer(tb testing.TB, image string, port int, opts ...testcontainers.ContainerCustomizer) (c *testcontainers.DockerContainer, addr string) {
	tb.Helper()

	if testing.Short() {
		tb.Skip("Skipping testcontainer test in short mode")
	}

	if os.Getenv("SKIP_TESTCONTAINERS") != "" {
		tb.Skip("Skipping testcontainer test because SKIP_TESTCONTAINERS is set")
	}

	ctx := tb.Context()

	portStr := strconv.Itoa(port) + "/tcp"

	opts = append(opts,
		testcontainers.WithExposedPorts(portStr),
		testcontainers.WithLogger(tLogger{tb}),
		testcontainers.WithAdditionalWaitStrategy(
			wait.ForListeningPort(nat.Port(portStr)),
		),
	)

	c, err := testcontainers.Run(ctx, image, opts...)
	testcontainers.CleanupContainer(tb, c)
	require.NoError(tb, err)

	mappedPort, err := c.MappedPort(ctx, nat.Port(portStr))
	require.NoError(tb, err)
	mappedIP, err := c.Host(ctx)
	require.NoError(tb, err)

	mappedAddr := (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(mappedIP, mappedPort.Port()),
	}).String()

	return c, mappedAddr
}
