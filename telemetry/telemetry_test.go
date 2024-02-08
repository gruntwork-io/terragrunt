package telemetry

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestInitTelemetryDisabled(t *testing.T) {
	opts := &TelemetryOptions{
		Vars: map[string]string{
			"TERRAGRUNT_TELEMETRY_ENABLED": "false",
		},
	}
	err := InitTelemetry(context.Background(), opts)
	assert.NoError(t, err)
}

func TestInitTelemetryEnabledConsole(t *testing.T) {
	opts := &TelemetryOptions{
		Vars: map[string]string{
			"TERRAGRUNT_TELEMETRY_ENABLED":                "true",
			"TERRAGRUNT_TELEMETRY_EXPORTER":               "console",
			"TERRAGRUNT_TELEMERTY_EXPORTER_HTTP_ENDPOINT": "http://example.com",
		},
		AppName:    "testApp",
		AppVersion: "1.0",
	}
	err := InitTelemetry(context.Background(), opts)
	assert.NoError(t, err)
}

func TestShutdownTelemetry(t *testing.T) {
	err := ShutdownTelemetry(context.Background())
	assert.NoError(t, err)
}

func TestTraceFunctionExecution(t *testing.T) {
	opts := &options.TerragruntOptions{
		CtxTelemetryCtx: context.Background(),
	}
	name := "testSpan"
	attrs := map[string]interface{}{
		"key1": "value1",
	}

	err := Trace(opts, name, attrs, func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err)
}
