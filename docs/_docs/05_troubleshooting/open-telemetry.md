---
layout: collection-browser-doc
title: OpenTelemetry
category: troubleshooting
categories_url: troubleshooting
excerpt: Learn how to debug issues with terragrunt and tofu/terraform.
tags: ["OpenTelemetry", "Tracing", "Metrics", "Otel"]
order: 502
nav_title: Documentation
nav_title_link: /docs/
---

Terragrunt can be configured to emit telemetry in [OpenTelemetry](https://opentelemetry.io/) format, traces and metrics.

Concepts:

- [OpenTelemetry](https://opentelemetry.io/)
- [Traces](https://opentelemetry.io/docs/concepts/signals/traces/)
- [Metrics](https://opentelemetry.io/docs/concepts/signals/metrics/)
- [Jaeger](https://www.jaegertracing.io/)

Tracing configuration:

- `TERRAGRUNT_TELEMETRY_TRACE_EXPORTER` - traces exporter type to be used. Currently supported values are:
  - `none` - no trace exporting, default value.
  - `console` - to export traces to console as JSON
  - `otlpHttp` - to export traces to an OpenTelemetry collector over HTTP [otlptracehttp](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp)
  - `otlpGrpc` - to export traces over gRPC [otlptracegrpc](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc)
  - `http` - to export traces to a custom HTTP endpoint using [otlptracehttp](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp)
- `TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT` - in case of `http` exporter, this is the endpoint to which traces will be sent.
- `TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_INSECURE_ENDPOINT` - if set to true, the exporter will not validate the server's certificate, helpful for local traces collection.
- `TRACEPARENT` - if set, the value will be used as a parent trace context, format `TRACEPARENT=00-<hex_trace_id>-<hex_span_id>-<trace_flags>`, example: `TRACEPARENT=00-xxx-yyy-01`

Metrics configuration:

- `TERRAGRUNT_TELEMETRY_METRIC_EXPORTER` - metrics exporter type to be used. Currently supported values are:
  - `none` - no metric exporting, default value.
  - `console` - write metrics to console as JSONs.
  - `otlpHttp` - export metrics to an OpenTelemetry collector over HTTP [otlpmetrichttp](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp)
  - `grpcHttp` - export metrics to an OpenTelemetry collector over gRPC [otlpmetricgrpc](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc)
- `TERRAGRUNT_TELEMETRY_METRIC_EXPORTER_INSECURE_ENDPOINT` - if set to true, the exporter will not validate the server's certificate, helpful for local metrics collection.

## Example configurations for trace collection

Collection of examples how to configure Terragrunt to emit traces and metrics in OpenTelemetry format.

## Example traces collection with Jaeger

- Start a Jaeger instance with docker:

```bash
docker run --rm --name jaeger -e COLLECTOR_OTLP_ENABLED=true -p 16686:16686 -p 4317:4317 -p 4318:4318 jaegertracing/all-in-one:1.54.0
```

- Verify that UI is available at <http://localhost:16686/>
- Define environment variables for Terragrunt to report traces to Jaeger:

```bash
export TERRAGRUNT_TELEMETRY_TRACE_EXPORTER=http
export TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT=localhost:4318
export TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_INSECURE_ENDPOINT=true
```

- Run terragrunt
- Verify that traces are available in Jaeger UI

## Configurations to collect traces in Grafana Tempo

- Start a Grafana Tempo instance [example](https://grafana.com/docs/tempo/latest/getting-started/docker-example/)
- Define environment variables for Terragrunt to report traces to Tempo:

```bash
export TERRAGRUNT_TELEMETRY_TRACE_EXPORTER=otlpHttp
# Replace with your tempo instance
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_INSECURE_ENDPOINT=true
````

- Run terragrunt
- Check for traces in Tempo UI for service "terragrunt"

## Example traces collection in console

- Set env variable to enable telemetry:

```bash
export TERRAGRUNT_TELEMETRY_TRACE_EXPORTER=console
```

- Run terragrunt
- Check produced traces in console, like:

```json
{"Name":"run_bash","SpanContext":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"f91587247524593b","TraceFlags":"01","TraceState":"","Remote":false},"Parent":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"b0b007770f852066","TraceFlags":"01","TraceState":"","Remote":false},"SpanKind":1,"StartTime":"2024-02-08T12:32:30.564217484Z","EndTime":"2024-02-08T12:32:31.570666395Z","Attributes":[{"Key":"command","Value":{"Type":"STRING","Value":"bash"}},{"Key":"args","Value":{"Type":"STRING","Value":"[-c sleep 1]"}},{"Key":"dir","Value":{"Type":"STRING","Value":"/projects/gruntwork/terragrunt-tests/trace-test/mod2"}}],"Events":null,"Links":null,"Status":{"Code":"Unset","Description":""},"DroppedAttributes":0,"DroppedEvents":0,"DroppedLinks":0,"ChildSpanCount":0,"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"terragrunt"}},{"Key":"service.version","Value":{"Type":"STRING","Value":"v0.55.0-29-g66bfa07b756e-dirty"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.23.0"}}],"InstrumentationLibrary":{"Name":"terragrunt","Version":"","SchemaURL":""}}
{"Name":"parse_config_file","SpanContext":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"d2823047fb469bdf","TraceFlags":"01","TraceState":"","Remote":false},"Parent":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"b0b007770f852066","TraceFlags":"01","TraceState":"","Remote":false},"SpanKind":1,"StartTime":"2024-02-08T12:32:30.380054129Z","EndTime":"2024-02-08T12:32:31.570899286Z","Attributes":[{"Key":"config_path","Value":{"Type":"STRING","Value":"/projects/gruntwork/terragrunt-tests/trace-test/mod2/terragrunt.hcl"}}],"Events":null,"Links":null,"Status":{"Code":"Unset","Description":""},"DroppedAttributes":0,"DroppedEvents":0,"DroppedLinks":0,"ChildSpanCount":0,"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"terragrunt"}},{"Key":"service.version","Value":{"Type":"STRING","Value":"v0.55.0-29-g66bfa07b756e-dirty"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.23.0"}}],"InstrumentationLibrary":{"Name":"terragrunt","Version":"","SchemaURL":""}}
{"Name":"run_terraform","SpanContext":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"152d873a18559f07","TraceFlags":"01","TraceState":"","Remote":false},"Parent":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"b0b007770f852066","TraceFlags":"01","TraceState":"","Remote":false},"SpanKind":1,"StartTime":"2024-02-08T12:32:31.57161757Z","EndTime":"2024-02-08T12:32:31.688157882Z","Attributes":[{"Key":"command","Value":{"Type":"STRING","Value":"tofu"}},{"Key":"args","Value":{"Type":"STRING","Value":"[init]"}},{"Key":"dir","Value":{"Type":"STRING","Value":"/projects/gruntwork/terragrunt-tests/trace-test/mod2"}}],"Events":null,"Links":null,"Status":{"Code":"Unset","Description":""},"DroppedAttributes":0,"DroppedEvents":0,"DroppedLinks":0,"ChildSpanCount":0,"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"terragrunt"}},{"Key":"service.version","Value":{"Type":"STRING","Value":"v0.55.0-29-g66bfa07b756e-dirty"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.23.0"}}],"InstrumentationLibrary":{"Name":"terragrunt","Version":"","SchemaURL":""}}
{"Name":"run_terraform","SpanContext":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"29341bdb65f66b1e","TraceFlags":"01","TraceState":"","Remote":false},"Parent":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"b0b007770f852066","TraceFlags":"01","TraceState":"","Remote":false},"SpanKind":1,"StartTime":"2024-02-08T12:32:31.688240673Z","EndTime":"2024-02-08T12:32:31.793377642Z","Attributes":[{"Key":"command","Value":{"Type":"STRING","Value":"tofu"}},{"Key":"args","Value":{"Type":"STRING","Value":"[apply -auto-approve -input=false]"}},{"Key":"dir","Value":{"Type":"STRING","Value":"/projects/gruntwork/terragrunt-tests/trace-test/mod2"}}],"Events":null,"Links":null,"Status":{"Code":"Unset","Description":""},"DroppedAttributes":0,"DroppedEvents":0,"DroppedLinks":0,"ChildSpanCount":0,"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"terragrunt"}},{"Key":"service.version","Value":{"Type":"STRING","Value":"v0.55.0-29-g66bfa07b756e-dirty"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.23.0"}}],"InstrumentationLibrary":{"Name":"terragrunt","Version":"","SchemaURL":""}}
{"Name":"run_module","SpanContext":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"8a01522bc65e0f1b","TraceFlags":"01","TraceState":"","Remote":false},"Parent":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"b0b007770f852066","TraceFlags":"01","TraceState":"","Remote":false},"SpanKind":1,"StartTime":"2024-02-08T12:32:30.290680776Z","EndTime":"2024-02-08T12:32:31.793392803Z","Attributes":[{"Key":"path","Value":{"Type":"STRING","Value":"/projects/gruntwork/terragrunt-tests/trace-test/mod2"}},{"Key":"terraformCommand","Value":{"Type":"STRING","Value":"apply"}}],"Events":null,"Links":null,"Status":{"Code":"Unset","Description":""},"DroppedAttributes":0,"DroppedEvents":0,"DroppedLinks":0,"ChildSpanCount":0,"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"terragrunt"}},{"Key":"service.version","Value":{"Type":"STRING","Value":"v0.55.0-29-g66bfa07b756e-dirty"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.23.0"}}],"InstrumentationLibrary":{"Name":"terragrunt","Version":"","SchemaURL":""}}
{"Name":"run-all apply","SpanContext":{"TraceID":"bdf3cb9078706b7f0b4f1d92428eedc0","SpanID":"b0b007770f852066","TraceFlags":"01","TraceState":"","Remote":false},"Parent":{"TraceID":"00000000000000000000000000000000","SpanID":"0000000000000000","TraceFlags":"00","TraceState":"","Remote":false},"SpanKind":1,"StartTime":"2024-02-08T12:32:26.388519019Z","EndTime":"2024-02-08T12:32:31.793405603Z","Attributes":[{"Key":"terraformCommand","Value":{"Type":"STRING","Value":"apply"}},{"Key":"args","Value":{"Type":"STRING","Value":"[apply]"}},{"Key":"dir","Value":{"Type":"STRING","Value":"/projects/gruntwork/terragrunt-tests/trace-test"}}],"Events":null,"Links":null,"Status":{"Code":"Unset","Description":""},"DroppedAttributes":0,"DroppedEvents":0,"DroppedLinks":0,"ChildSpanCount":28,"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"terragrunt"}},{"Key":"service.version","Value":{"Type":"STRING","Value":"v0.55.0-29-g66bfa07b756e-dirty"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.23.0"}}],"InstrumentationLibrary":{"Name":"terragrunt","Version":"","SchemaURL":""}}
```

## Collection of metrics with OpenTelemetry collector and Prometheus

- Start OpenTelemetry collector with Prometheus receiver.

  Example setup through `docker-compose.yml`:

  ```yaml
  version: '3'
  services:
    otel-collector:
      image: otel/opentelemetry-collector:0.94.0
      volumes:
        - ./otel-collector-config.yaml:/etc/otelcol/config.yaml
      ports:
        - "4317:4317" # OTLP gRPC receiver
        - "4318:4318" # OTLP HTTP receiver
        - "8889:8889" # Prometheus exporter
    prometheus:
      image: prom/prometheus:v2.45.3
      volumes:
        - ./prometheus.yml:/etc/prometheus/prometheus.yml
      ports:
        - "9090:9090"
      depends_on:
        - otel-collector
  ```

OpenTelemetry collector configuration `otel-collector-config.yaml`:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
      http:

processors:
  batch:

exporters:
  prometheus:
    endpoint: "0.0.0.0:8889" # Prometheus exporter endpoint

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus]
```

Prometheus configuration file `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'opentelemetry'
    scrape_interval: 5s
    static_configs:
      - targets: ['otel-collector:8889']

```

- Confirm that Prometheus is available at <http://localhost:9090/>
- Define environment variables for Terragrunt to report metrics to OpenTelemetry collector:

```bash
export TERRAGRUNT_TELEMETRY_METRIC_EXPORTER=grpcHttp
export TERRAGRUNT_TELEMETRY_METRIC_EXPORTER_INSECURE_ENDPOINT=true
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

- Run terragrunt
- Verify that metrics are available in Prometheus UI

Example configuration to export metrics to console:

- Set env variable to enable telemetry:

```bash
export TERRAGRUNT_TELEMETRY_METRIC_EXPORTER=console
```

- Run terragrunt
- In output will be printed messages like:

```json
{"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"terragrunt"}},{"Key":"service.version","Value":{"Type":"STRING","Value":"v0.55.0-41-g7185318bb11b"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.23.1"}}],"ScopeMetrics":[]}
{"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"terragrunt"}},{"Key":"service.version","Value":{"Type":"STRING","Value":"v0.55.0-41-g7185318bb11b"}},{"Key":"telemetry.sdk.language","Value":{"Type":"STRING","Value":"go"}},{"Key":"telemetry.sdk.name","Value":{"Type":"STRING","Value":"opentelemetry"}},{"Key":"telemetry.sdk.version","Value":{"Type":"STRING","Value":"1.23.1"}}],"ScopeMetrics":[{"Scope":{"Name":"terragrunt","Version":"","SchemaURL":""},"Metrics":[{"Name":"run_bash_duration","Description":"","Unit":"","Data":{"DataPoints":[{"Attributes":[{"Key":"args","Value":{"Type":"STRING","Value":"[-c sleep 2]"}},{"Key":"command","Value":{"Type":"STRING","Value":"bash"}},{"Key":"dir","Value":{"Type":"STRING","Value":"/projects/gruntwork/terragrunt-tests/trace-test/mod3"}}],"StartTime":"2024-02-12T14:38:14.85578658Z","Time":"2024-02-12T14:38:17.853165589Z","Count":1,"Bounds":[0,5,10,25,50,75,100,250,500,750,1000,2500,5000,7500,10000],"BucketCounts":[0,0,0,0,0,0,0,0,0,0,0,1,0,0,0,0],"Min":2005,"Max":2005,"Sum":2005}],"Temporality":"CumulativeTemporality"}},{"Name":"run_bash_success_count","Description":"","Unit":"","Data":{"DataPoints":[{"Attributes":[],"StartTime":"2024-02-12T14:38:16.860878555Z","Time":"2024-02-12T14:38:17.853169359Z","Value":1}],"Temporality":"CumulativeTemporality","IsMonotonic":true}}]}]}
```
