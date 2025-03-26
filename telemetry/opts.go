package telemetry

// Options are Telemetry options.
type Options struct { //nolint: govet
	// TraceExporter is the type of trace exporter to be used.
	TraceExporter string
	// TraceExporterInsecureEndpoint is useful for collecting traces locally. If set to true, the exporter will not validate the server certificate.
	TraceExporterInsecureEndpoint bool
	// TraceExporterHTTPEndpoint is the endpoint to which traces will be sent.
	TraceExporterHTTPEndpoint string
	// TraceParent is used as a parent trace context.
	TraceParent string
	// MetricExporter is the type of  metrics exporter.
	MetricExporter string
	// MetricExporterInsecureEndpoint is useful for local metrics collection. if set to true, the exporter will not validate the server's certificate.
	MetricExporterInsecureEndpoint bool
}
