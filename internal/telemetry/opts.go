package telemetry

import (
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// Options are Telemetry options.
type Options struct {
	// TraceExporter is the type of trace exporter to be used.
	TraceExporter string
	// TraceExporterHTTPEndpoint is the endpoint to which traces will be sent.
	TraceExporterHTTPEndpoint string
	// TraceParent is used as a parent trace context.
	TraceParent string
	// TraceRootSpanKind overrides the OpenTelemetry span kind of the root command
	// span. Empty (the default) leaves it as "internal", which is what the
	// OpenTelemetry specification prescribes for the local root span of a CLI
	// process.
	//
	// Several APM backends (New Relic, Datadog, Elastic) only derive a
	// service-level transaction - throughput, response time, error rate - from
	// entry spans whose kind is "server" or "consumer". Setting this to "server"
	// makes each Terragrunt run show up as such a transaction without needing an
	// OpenTelemetry Collector in the export path to rewrite it.
	TraceRootSpanKind string
	// MetricExporter is the type of metrics exporter.
	MetricExporter string
	// LogsExporter is the type of logs exporter to be used.
	LogsExporter string
	// TraceExporterInsecureEndpoint is useful for collecting traces locally.
	// If set to true, the exporter will not validate the server certificate.
	TraceExporterInsecureEndpoint bool
	// MetricExporterInsecureEndpoint is useful for local metrics collection.
	// If set to true, the exporter will not validate the server's certificate.
	MetricExporterInsecureEndpoint bool
	// LogsExporterInsecureEndpoint is useful for local logs collection.
	// If set to true, the exporter will not validate the server's certificate.
	LogsExporterInsecureEndpoint bool
}

// RootSpanKind resolves TraceRootSpanKind to an OpenTelemetry span kind.
// Unset, unrecognized and nil-receiver cases all yield [trace.SpanKindInternal],
// so a bad value degrades to the default behavior rather than failing a run.
func (opts *Options) RootSpanKind() trace.SpanKind {
	if opts == nil {
		return trace.SpanKindInternal
	}

	switch strings.ToLower(strings.TrimSpace(opts.TraceRootSpanKind)) {
	case "server":
		return trace.SpanKindServer
	case "consumer":
		return trace.SpanKindConsumer
	case "producer":
		return trace.SpanKindProducer
	case "client":
		return trace.SpanKindClient
	default:
		return trace.SpanKindInternal
	}
}
