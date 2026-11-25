package telemetry_test

import (
	"compress/gzip"
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
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

			opts := options.NewTerragruntOptions()
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

	opts := options.NewTerragruntOptions()
	opts.Telemetry.LogsExporter = "console"

	logger, err := telemetry.NewLogger(ctx, "terragrunt", "test", io.Discard, opts.Telemetry, nil)
	require.NoError(t, err)
	require.NotNil(t, logger)

	opts.Telemetry.LogsExporter = "none"

	logger, err = telemetry.NewLogger(ctx, "terragrunt", "test", io.Discard, opts.Telemetry, nil)
	require.NoError(t, err)
	assert.Nil(t, logger)
}

func TestNewLogsExporterRejectsUnknownType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		exporterType string
	}{
		{name: "lower case", exporterType: "otlphttp"},
		{name: "metrics name for gRPC", exporterType: "grpcHttp"},
		{name: "short name", exporterType: "otlp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			opts.Telemetry.LogsExporter = tt.exporterType

			exporter, err := telemetry.NewLogsExporter(t.Context(), io.Discard, opts.Telemetry)

			var unknown *telemetry.ErrorUnknownLogsExporter

			require.ErrorAs(t, err, &unknown)
			assert.Equal(t, tt.exporterType, unknown.Value)
			assert.Nil(t, exporter)
		})
	}
}

func TestNewLoggerRejectsInvalidBatchSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		envName string
		value   string
	}{
		{name: "queue size is not a number", envName: "OTEL_BLRP_MAX_QUEUE_SIZE", value: "many"},
		{name: "queue size is zero", envName: "OTEL_BLRP_MAX_QUEUE_SIZE", value: "0"},
		{name: "batch size is not a number", envName: "OTEL_BLRP_MAX_EXPORT_BATCH_SIZE", value: "1k"},
		{name: "batch size is negative", envName: "OTEL_BLRP_MAX_EXPORT_BATCH_SIZE", value: "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			opts.Telemetry.LogsExporter = "console"

			logger, err := telemetry.NewLogger(
				t.Context(), "terragrunt", "test", io.Discard, opts.Telemetry, map[string]string{tt.envName: tt.value},
			)

			var invalid *telemetry.ErrorInvalidEnvVariable

			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, tt.envName, invalid.Name)
			assert.Equal(t, tt.value, invalid.Value)
			assert.Nil(t, logger)
		})
	}
}

// TestOTLPLogsExport checks what a fake collector receives from each OTLP exporter.
func TestOTLPLogsExport(t *testing.T) {
	tests := []struct {
		newCollector func(t *testing.T) *otlpCollector
		name         string
		exporter     string
		compression  string
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
		{
			name:         "otlpHttp with gzip",
			exporter:     "otlpHttp",
			newCollector: func(t *testing.T) *otlpCollector { t.Helper(); return newHTTPCollector(t) },
			compression:  "gzip",
		},
		{
			name:         "otlpHttp with a private CA",
			exporter:     "otlpHttp",
			newCollector: newTLSCollector,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := tt.newCollector(t)
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.endpoint)
			t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer%20test-token")
			t.Setenv("OTEL_EXPORTER_OTLP_COMPRESSION", tt.compression)

			for name, value := range collector.env {
				t.Setenv(name, value)
			}

			opts := options.NewTerragruntOptions()
			opts.Telemetry.TraceExporter = consoleExporter
			opts.Telemetry.LogsExporter = tt.exporter
			opts.Telemetry.LogsExporterInsecureEndpoint = strings.HasPrefix(collector.endpoint, "http://")

			l := discardLogger()
			tlm := newTestTelemeter(t, l, opts, nil)

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
			assert.Equal(t, tt.compression, collector.contentEncoding())

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

			// A logger without a span uses the root command span.
			unboundRecord, ok := byBody["unbound record"]
			require.True(t, ok, "unbound record not exported: %v", records)
			assert.Equal(t, logspb.SeverityNumber_SEVERITY_NUMBER_INFO, unboundRecord.severity)
			assert.Equal(t, commandSpan.TraceID(), unboundRecord.traceID)
			assert.Equal(t, commandSpan.SpanID(), unboundRecord.spanID)
		})
	}
}

// TestOTLPLogsExportBatchesLargeVolume checks that records spread over several batches all arrive once.
func TestOTLPLogsExportBatchesLargeVolume(t *testing.T) {
	// 1500 records fill several 512 record batches and fit in the 2048 record queue.
	const (
		goroutines   = 10
		perGoroutine = 150
		maxBatchSize = 512
		maxQueueSize = 2048
		total        = goroutines * perGoroutine
	)

	collector := newHTTPCollector(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.endpoint)

	opts := options.NewTerragruntOptions()
	opts.Telemetry.LogsExporter = "otlpHttp"
	opts.Telemetry.LogsExporterInsecureEndpoint = true

	l := discardLogger()
	tlm := newTestTelemeter(t, l, opts, map[string]string{
		"OTEL_BLRP_MAX_EXPORT_BATCH_SIZE": strconv.Itoa(maxBatchSize),
		"OTEL_BLRP_MAX_QUEUE_SIZE":        strconv.Itoa(maxQueueSize),
	})

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

// TestTelemeterShutdownFlushesLogsWhenMetricExportFails checks that a failed metrics flush does not drop logs.
func TestTelemeterShutdownFlushesLogsWhenMetricExportFails(t *testing.T) {
	collector := newHTTPCollector(t, "/v1/metrics")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.endpoint)

	opts := options.NewTerragruntOptions()
	opts.Telemetry.MetricExporter = "otlpHttp"
	opts.Telemetry.MetricExporterInsecureEndpoint = true
	opts.Telemetry.LogsExporter = "otlpHttp"
	opts.Telemetry.LogsExporterInsecureEndpoint = true

	l := discardLogger()
	tlm := newTestTelemeter(t, l, opts, nil)

	tlm.Count(t.Context(), "test_metric", 1)
	l.Infof("record before shutdown")

	require.Error(t, tlm.Shutdown(t.Context()))

	records := collector.records()
	require.Len(t, records, 1)
	assert.Equal(t, "record before shutdown", records[0].body)
}

// TestTelemeterShutdownFlushesLogsWhenTraceExportHangs checks that a hung trace flush does not drop logs.
func TestTelemeterShutdownFlushesLogsWhenTraceExportHangs(t *testing.T) {
	collector := newHTTPCollector(t)
	traceCollector := newHangingCollector(t)

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", traceCollector.URL+"/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", collector.endpoint+"/v1/logs")
	t.Setenv("OTEL_BLRP_SCHEDULE_DELAY", "60000")

	opts := options.NewTerragruntOptions()
	opts.Telemetry.TraceExporter = "otlpHttp"
	opts.Telemetry.TraceExporterInsecureEndpoint = true
	opts.Telemetry.LogsExporter = "otlpHttp"
	opts.Telemetry.LogsExporterInsecureEndpoint = true

	l := discardLogger()
	tlm := newTestTelemeter(t, l, opts, nil)

	require.NoError(t, tlm.Collect(t.Context(), l, "test_span", nil, func(_ context.Context, childL log.Logger) error {
		childL.Infof("record before shutdown")

		return nil
	}))

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	require.ErrorIs(t, tlm.Shutdown(ctx), context.DeadlineExceeded)

	records := collector.records()
	require.Len(t, records, 1)
	assert.Equal(t, "record before shutdown", records[0].body)
}

func TestTelemeterShutdownReportsLogRecordsOfFailedExport(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.Telemetry.LogsExporter = consoleExporter

	l := discardLogger()

	tlm, err := telemetry.NewTelemeter(
		t.Context(), l, "terragrunt", "v0.0.0-test", failedWriter{}, opts.Telemetry, nil, true,
	)
	require.NoError(t, err)

	const total = 3

	for i := range total {
		l.Infof("record %d", i)
	}

	var lost *telemetry.ErrorLogRecordsNotExported

	require.ErrorAs(t, tlm.Shutdown(t.Context()), &lost)
	assert.Equal(t, uint64(total), lost.Emitted)
	assert.Equal(t, uint64(total), lost.Lost)
}

func TestTelemeterShutdownReportsLogRecordsDroppedFromFullQueue(t *testing.T) {
	t.Parallel()

	const (
		total     = 200
		queueSize = 8
	)

	opts := options.NewTerragruntOptions()
	opts.Telemetry.LogsExporter = consoleExporter

	l := discardLogger()
	writer := &heldWriter{release: make(chan struct{})}

	tlm, err := telemetry.NewTelemeter(
		t.Context(), l, "terragrunt", "v0.0.0-test", writer, opts.Telemetry,
		map[string]string{"OTEL_BLRP_MAX_QUEUE_SIZE": strconv.Itoa(queueSize)}, true,
	)
	require.NoError(t, err)

	for i := range total {
		l.Infof("record %d", i)
	}

	close(writer.release)

	var lost *telemetry.ErrorLogRecordsNotExported

	require.ErrorAs(t, tlm.Shutdown(t.Context()), &lost)
	assert.Equal(t, uint64(total), lost.Emitted)
	assert.Positive(t, lost.Lost)
	assert.Equal(t, uint64(total), lost.Lost+writer.records())
}

// otlpCollector is a fake collector that records OTLP log exports over HTTP or gRPC.
type otlpCollector struct {
	collogspb.UnimplementedLogsServiceServer
	failPaths map[string]bool
	env       map[string]string
	endpoint  string
	encoding  string
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

// newHTTPCollector starts a fake OTLP/HTTP collector that fails requests to failPaths.
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

// newTLSCollector starts a fake OTLP/HTTP collector with a certificate that only its env trusts.
func newTLSCollector(t *testing.T) *otlpCollector {
	t.Helper()

	collector := &otlpCollector{failPaths: map[string]bool{}}

	srv := httptest.NewTLSServer(collector)
	t.Cleanup(srv.Close)

	certFile := filepath.Join(t.TempDir(), "ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))

	collector.endpoint = srv.URL
	collector.env = map[string]string{"OTEL_EXPORTER_OTLP_CERTIFICATE": certFile}

	return collector
}

// newHangingCollector starts an OTLP/HTTP server that never answers until the client gives up.
func newHangingCollector(t *testing.T) *httptest.Server {
	t.Helper()

	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))

	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	return srv
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
	c.record(metadata.ValueFromIncomingContext(ctx, "authorization"), "", req)

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

	encoding := r.Header.Get("Content-Encoding")

	var reader io.Reader = r.Body

	if encoding == "gzip" {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		reader = zr
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req := &collogspb.ExportLogsServiceRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.record(r.Header.Values("Authorization"), encoding, req)

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

func (c *otlpCollector) record(auth []string, encoding string, req *collogspb.ExportLogsServiceRequest) {
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
	c.encoding = encoding
	c.batches = append(c.batches, batch)
}

func (c *otlpCollector) contentEncoding() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.encoding
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

// discardLogger returns a debug level logger that writes nothing.
func discardLogger() log.Logger {
	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	return l
}

func newTestTelemeter(
	t *testing.T,
	l log.Logger,
	opts *options.TerragruntOptions,
	env map[string]string,
) *telemetry.Telemeter {
	t.Helper()

	tlm, err := telemetry.NewTelemeter(
		t.Context(), l, "terragrunt", "v0.0.0-test", io.Discard, opts.Telemetry, env, true,
	)
	require.NoError(t, err)

	return tlm
}

// failedWriter is a writer that fails each write.
type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

// heldWriter is a writer that blocks until release closes, then counts the console records.
type heldWriter struct {
	release chan struct{}
	written uint64
	mu      sync.Mutex
}

func (w *heldWriter) Write(p []byte) (int, error) {
	<-w.release

	w.mu.Lock()
	defer w.mu.Unlock()

	w.written += uint64(strings.Count(string(p), "\n"))

	return len(p), nil
}

func (w *heldWriter) records() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.written
}
