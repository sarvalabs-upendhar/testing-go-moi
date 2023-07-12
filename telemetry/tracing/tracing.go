package tracing

import (
	"context"
	"fmt"

	id "github.com/sarvalabs/go-moi/common/kramaid"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	traceapi "go.opentelemetry.io/otel/trace"
)

// shutdownTracerProvider adds a shutdown method for tracer providers.
type shutdownTracerProvider interface {
	Tracer(instrumentationName string, opts ...traceapi.TracerOption) traceapi.Tracer
	Shutdown(ctx context.Context) error
}

// noopShutdownTracerProvider adds a no-op Shutdown method to a TracerProvider.
type noopShutdownTracerProvider struct{ traceapi.TracerProvider }

func (n *noopShutdownTracerProvider) Shutdown(ctx context.Context) error { return nil }

func buildExporters(ctx context.Context, jaegerAddress string) ([]trace.SpanExporter, error) {
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

	// ** Jaeger Exporter
	jaegerExporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(jaegerAddress)))
	if err != nil {
		return nil, fmt.Errorf("building Jaeger exporter: %w", err)
	}

	exporters = append(exporters, jaegerExporter)

	return exporters, nil
}

// TODO: we're adding an unexported type in an exported func(fix)

// NewTracerProvider creates and configures a TracerProvider.
func NewTracerProvider(
	ctx context.Context,
	enableTracing bool,
	jaegerAddress string,
	kramaID id.KramaID,
) (shutdownTracerProvider, error) {
	if !enableTracing {
		return &noopShutdownTracerProvider{TracerProvider: traceapi.NewNoopTracerProvider()}, nil
	}

	exporters, err := buildExporters(ctx, jaegerAddress)
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
			semconv.ServiceNameKey.String("moi-chain"),
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
