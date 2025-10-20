package tracing

import (
	"context"
	"fmt"

	"github.com/sarvalabs/go-moi/common/identifiers"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	traceapi "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/sarvalabs/go-moi/common/config"
)

// shutdownTracerProvider adds a shutdown method for tracer providers.
type shutdownTracerProvider interface {
	embedded.TracerProvider
	Tracer(instrumentationName string, opts ...traceapi.TracerOption) traceapi.Tracer
	Shutdown(ctx context.Context) error
}

// noopShutdownTracerProvider adds a no-op Shutdown method to a TracerProvider.
type noopShutdownTracerProvider struct{ traceapi.TracerProvider }

func (n *noopShutdownTracerProvider) Shutdown(ctx context.Context) error { return nil }

func buildExporters(ctx context.Context, otlpAddress, token string) ([]trace.SpanExporter, error) {
	var exporters []trace.SpanExporter
	// *** File Exporter
	// filePath := ""
	// if filePath == "" {
	// 	cwd, err := os.Getwd()
	// 	if err != nil {
	// 		return nil, fmt.Errorf("finding working directory for the OpenTelemetry file exporter: %w", err)
	// 	}
	// 	filePath = path.Join(cwd, "traces.json")
	// }
	// exporter, err := newFileExporter(filePath)
	// if err != nil {
	// 	return nil, err
	// }
	// exporters = append(exporters, exporter)

	// ** OTLP Exporter
	otlpExporterOpts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(otlpAddress),
	}

	if token != "" {
		otlpExporterOpts = append(otlpExporterOpts, otlptracehttp.WithHeaders(map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", token),
		}))
	} else {
		otlpExporterOpts = append(otlpExporterOpts, otlptracehttp.WithInsecure())
	}

	otlpExporter, err := otlptracehttp.New(ctx, otlpExporterOpts...)
	if err != nil {
		return nil, fmt.Errorf("building OTLP exporter: %w", err)
	}

	exporters = append(exporters, otlpExporter)

	return exporters, nil
}

// TODO: we're adding an unexported type in an exported func(fix)

// NewTracerProvider creates and configures a TracerProvider.
func NewTracerProvider(
	ctx context.Context,
	enableTracing bool,
	otlpAddress, token, networkID string,
	kramaID identifiers.KramaID,
) (shutdownTracerProvider, error) {
	if !enableTracing {
		return &noopShutdownTracerProvider{TracerProvider: noop.NewTracerProvider()}, nil
	}

	exporters, err := buildExporters(ctx, otlpAddress, token)
	if err != nil {
		return nil, err
	}

	options := []trace.TracerProviderOption{}

	for _, exporter := range exporters {
		options = append(options, trace.WithBatcher(exporter))
	}

	r, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceNameKey.String("moi-chain-"+config.ProtocolVersion+"-"+networkID),
			semconv.ServiceVersionKey.String("0.0.1"),
			attribute.String("krama-id", string(kramaID)),
		),
	)
	if err != nil {
		return nil, err
	}

	options = append(options, trace.WithResource(r))

	return trace.NewTracerProvider(options...), nil
}

func Span(ctx context.Context,
	componentName string,
	spanName string,
	opts ...traceapi.SpanStartOption,
) (context.Context, traceapi.Span) {
	return otel.Tracer("moi-chain").Start(ctx, fmt.Sprintf("%s.%s", componentName, spanName), opts...)
}
