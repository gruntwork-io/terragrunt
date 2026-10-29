package telemetry_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/trace"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func TestNewLogsExporter(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	stdout, err := stdoutlog.New()
	require.NoError(t, err)

	tests := []struct {
		expectedType any
		name         string
		exporterType string
		insecure     bool
		expectNil    bool
	}{
		{
			name:         "OTLP HTTP Exporter",
			exporterType: "otlpHttp",
			insecure:     false,
			expectedType: (*otlploghttp.Exporter)(nil),
		},
		{
			name:         "OTLP gRPC Exporter",
			exporterType: "otlpGrpc",
			insecure:     true,
			expectedType: (*otlploggrpc.Exporter)(nil),
		},
		{
			name:         "Console Exporter",
			exporterType: "console",
			insecure:     false,
			expectedType: stdout,
		},
		{
			name:         "None Exporter",
			exporterType: "none",
			insecure:     false,
			expectNil:    true,
		},
		{
			name:         "Empty Exporter",
			exporterType: "",
			insecure:     false,
			expectNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.Telemetry.LogsExporter = tt.exporterType
			opts.Telemetry.LogsExporterInsecureEndpoint = tt.insecure

			exporter, err := telemetry.NewLogsExporter(ctx, io.Discard, opts.Telemetry)
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, exporter)
				return
			}

			assert.IsType(t, tt.expectedType, exporter)
		})
	}
}

func TestNewLogger(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.LogsExporter = "console"

	logger, err := telemetry.NewLogger(ctx, "terragrunt", "test", io.Discard, opts.Telemetry)
	require.NoError(t, err)
	require.NotNil(t, logger)

	opts.Telemetry.LogsExporter = "none"

	logger, err = telemetry.NewLogger(ctx, "terragrunt", "test", io.Discard, opts.Telemetry)
	require.NoError(t, err)
	assert.Nil(t, logger)
}

// TestOTLPLogsExport sends records through each OTLP exporter to a fake
// collector and checks what the collector decodes from the wire.
func TestOTLPLogsExport(t *testing.T) {
	tests := []struct {
		newCollector func(t *testing.T) *otlpCollector
		name         string
		exporter     string
	}{
		{
			name:         "otlpHttp",
			exporter:     "otlpHttp",
			newCollector: func(t *testing.T) *otlpCollector { t.Helper(); return newHTTPCollector(t) },
		},
		{
			name:         "otlpGrpc",
			exporter:     "otlpGrpc",
			newCollector: newGRPCCollector,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := tt.newCollector(t)
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.endpoint)
			t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer%20test-token")

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.Telemetry.TraceExporter = consoleExporter
			opts.Telemetry.LogsExporter = tt.exporter
			opts.Telemetry.LogsExporterInsecureEndpoint = true

			l := discardLogger()
			tlm := newTestTelemeter(t, l, opts)

			var commandSpan, unitSpan trace.SpanContext

			require.NoError(t, tlm.Collect(t.Context(), l, "command", nil, func(ctx context.Context, _ log.Logger) error {
				commandSpan = trace.SpanContextFromContext(ctx)

				return tlm.Collect(ctx, l, "unit", nil, func(ctx context.Context, childL log.Logger) error {
					unitSpan = trace.SpanContextFromContext(ctx)

					childL.Errorf("unit record")
					l.Infof("unbound record")

					return nil
				})
			}))
			require.NoError(t, tlm.Shutdown(t.Context()))

			require.True(t, commandSpan.IsValid())
			require.True(t, unitSpan.IsValid())
			assert.Contains(t, collector.authorizations(), "Bearer test-token")

			records := collector.records()
			require.Len(t, records, 2)

			byBody := map[string]exportedLog{}
			for _, record := range records {
				byBody[record.body] = record
			}

			unitRecord, ok := byBody["unit record"]
			require.True(t, ok, "unit record not exported: %v", records)
			assert.Equal(t, logspb.SeverityNumber_SEVERITY_NUMBER_ERROR, unitRecord.severity)
			assert.Equal(t, "terragrunt", unitRecord.serviceName)
			assert.Equal(t, unitSpan.TraceID(), unitRecord.traceID)
			assert.Equal(t, unitSpan.SpanID(), unitRecord.spanID)

			// A logger that is not bound to a span falls back to the root command span.
			unboundRecord, ok := byBody["unbound record"]
			require.True(t, ok, "unbound record not exported: %v", records)
			assert.Equal(t, logspb.SeverityNumber_SEVERITY_NUMBER_INFO, unboundRecord.severity)
			assert.Equal(t, commandSpan.TraceID(), unboundRecord.traceID)
			assert.Equal(t, commandSpan.SpanID(), unboundRecord.spanID)
		})
	}
}

// TestOTLPLogsExportBatchesLargeVolume emits more records than one export
// batch holds, from many goroutines, and checks every record arrives once.
func TestOTLPLogsExportBatchesLargeVolume(t *testing.T) {
	// The SDK defaults are a 512 record batch and a 2048 record queue, so
	// 1500 records span several batches and none can be dropped.
	const (
		goroutines   = 10
		perGoroutine = 150
		maxBatchSize = 512
		total        = goroutines * perGoroutine
	)

	collector := newHTTPCollector(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.endpoint)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.LogsExporter = "otlpHttp"
	opts.Telemetry.LogsExporterInsecureEndpoint = true

	l := discardLogger()
	tlm := newTestTelemeter(t, l, opts)

	var wg sync.WaitGroup

	for g := range goroutines {
		wg.Go(func() {
			for i := range perGoroutine {
				l.Infof("record %d-%d", g, i)
			}
		})
	}

	wg.Wait()
	require.NoError(t, tlm.Shutdown(t.Context()))

	records := collector.records()
	require.Len(t, records, total)

	bodies := map[string]struct{}{}
	for _, record := range records {
		bodies[record.body] = struct{}{}
	}

	assert.Len(t, bodies, total, "every record must arrive exactly once")

	batches := collector.batchSizes()
	require.GreaterOrEqual(t, len(batches), total/maxBatchSize+1)

	for _, size := range batches {
		assert.LessOrEqual(t, size, maxBatchSize)
	}
}

// TestTelemeterShutdownFlushesLogsWhenMetricExportFails checks that a failed
// metrics flush at shutdown does not stop the buffered logs from exporting.
func TestTelemeterShutdownFlushesLogsWhenMetricExportFails(t *testing.T) {
	collector := newHTTPCollector(t, "/v1/metrics")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.endpoint)

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.Telemetry.MetricExporter = "otlpHttp"
	opts.Telemetry.MetricExporterInsecureEndpoint = true
	opts.Telemetry.LogsExporter = "otlpHttp"
	opts.Telemetry.LogsExporterInsecureEndpoint = true

	l := discardLogger()
	tlm := newTestTelemeter(t, l, opts)

	tlm.Count(t.Context(), "test_metric", 1)
	l.Infof("record before shutdown")

	require.Error(t, tlm.Shutdown(t.Context()))

	records := collector.records()
	require.Len(t, records, 1)
	assert.Equal(t, "record before shutdown", records[0].body)
}

// otlpCollector is a fake OpenTelemetry collector that records the OTLP log
// export requests it receives over HTTP or gRPC.
type otlpCollector struct {
	collogspb.UnimplementedLogsServiceServer
	failPaths map[string]bool
	endpoint  string
	auth      []string
	batches   [][]exportedLog
	mu        sync.Mutex
}

// exportedLog is one log record as the collector decoded it.
type exportedLog struct {
	body        string
	serviceName string
	severity    logspb.SeverityNumber
	traceID     trace.TraceID
	spanID      trace.SpanID
}

// newHTTPCollector starts a fake OTLP/HTTP collector that rejects requests to
// failPaths.
func newHTTPCollector(t *testing.T, failPaths ...string) *otlpCollector {
	t.Helper()

	collector := &otlpCollector{failPaths: map[string]bool{}}
	for _, path := range failPaths {
		collector.failPaths[path] = true
	}

	srv := httptest.NewServer(collector)
	t.Cleanup(srv.Close)

	collector.endpoint = srv.URL

	return collector
}

// newGRPCCollector starts a fake OTLP/gRPC collector.
func newGRPCCollector(t *testing.T) *otlpCollector {
	t.Helper()

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	collector := &otlpCollector{endpoint: "http://" + ln.Addr().String()}

	srv := grpc.NewServer()
	collogspb.RegisterLogsServiceServer(srv, collector)

	serveErr := make(chan error, 1)

	go func() {
		serveErr <- srv.Serve(ln)
	}()

	t.Cleanup(func() {
		srv.Stop()
		require.NoError(t, <-serveErr)
	})

	return collector
}

// Export implements the OTLP/gRPC logs service.
func (c *otlpCollector) Export(
	ctx context.Context,
	req *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	c.record(metadata.ValueFromIncomingContext(ctx, "authorization"), req)

	return &collogspb.ExportLogsServiceResponse{}, nil
}

// ServeHTTP implements the OTLP/HTTP endpoints, decoding only /v1/logs.
func (c *otlpCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if c.failPaths[r.URL.Path] {
		http.Error(w, "rejected by test collector", http.StatusBadRequest)
		return
	}

	if r.URL.Path != "/v1/logs" {
		w.WriteHeader(http.StatusOK)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req := &collogspb.ExportLogsServiceRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.record(r.Header.Values("Authorization"), req)

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

func (c *otlpCollector) record(auth []string, req *collogspb.ExportLogsServiceRequest) {
	var batch []exportedLog

	for _, resourceLogs := range req.GetResourceLogs() {
		serviceName := resourceAttribute(resourceLogs.GetResource(), "service.name")

		for _, scopeLogs := range resourceLogs.GetScopeLogs() {
			for _, record := range scopeLogs.GetLogRecords() {
				exported := exportedLog{
					body:        record.GetBody().GetStringValue(),
					serviceName: serviceName,
					severity:    record.GetSeverityNumber(),
				}

				copy(exported.traceID[:], record.GetTraceId())
				copy(exported.spanID[:], record.GetSpanId())

				batch = append(batch, exported)
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.auth = append(c.auth, auth...)
	c.batches = append(c.batches, batch)
}

func (c *otlpCollector) records() []exportedLog {
	c.mu.Lock()
	defer c.mu.Unlock()

	var records []exportedLog
	for _, batch := range c.batches {
		records = append(records, batch...)
	}

	return records
}

func (c *otlpCollector) batchSizes() []int {
	c.mu.Lock()
	defer c.mu.Unlock()

	sizes := make([]int, 0, len(c.batches))
	for _, batch := range c.batches {
		sizes = append(sizes, len(batch))
	}

	return sizes
}

func (c *otlpCollector) authorizations() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]string(nil), c.auth...)
}

func resourceAttribute(resource *resourcepb.Resource, key string) string {
	for _, attr := range resource.GetAttributes() {
		if attr.GetKey() == key {
			return attr.GetValue().GetStringValue()
		}
	}

	return ""
}

// discardLogger returns a debug level logger that writes nothing to the terminal.
func discardLogger() log.Logger {
	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	return l
}

func newTestTelemeter(t *testing.T, l log.Logger, opts *options.TerragruntOptions) *telemetry.Telemeter {
	t.Helper()

	tlm, err := telemetry.NewTelemeter(t.Context(), l, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry)
	require.NoError(t, err)

	return tlm
}
