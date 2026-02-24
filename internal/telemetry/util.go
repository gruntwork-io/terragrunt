package telemetry

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// mapToAttributes converts map to attributes to pass to span.SetAttributes.
func mapToAttributes(data map[string]any) []attribute.KeyValue {
	var attrs []attribute.KeyValue

	for k, v := range data {
		switch val := v.(type) {
		case string:
			attrs = append(attrs, attribute.String(k, val))
		case int:
			attrs = append(attrs, attribute.Int64(k, int64(val)))
		case int64:
			attrs = append(attrs, attribute.Int64(k, val))
		case float64:
			attrs = append(attrs, attribute.Float64(k, val))
		case bool:
			attrs = append(attrs, attribute.Bool(k, val))
		default:
			attrs = append(attrs, attribute.String(k, fmt.Sprintf("%v", val)))
		}
	}

	return attrs
}

// CleanMetricName cleans metric name from invalid characters.
func CleanMetricName(metricName string) string {
	cleanedName := metricNameCleanPattern.ReplaceAllString(metricName, "_")
	cleanedName = multipleUnderscoresPattern.ReplaceAllString(cleanedName, "_")

	return strings.Trim(cleanedName, "_")
}
