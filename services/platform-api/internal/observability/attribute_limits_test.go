package observability

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// GHSA-q7gw-24fr-5p69 bypassed span limits using valid replacement characters.
func TestSpanAttributeLimitCountsReplacementCharacters(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"replacement_characters", strings.Repeat("\uFFFD", 1000)},
		{"replacement_characters_with_ascii", strings.Repeat("\uFFFD", 100) + "abcdef"},
		{"ascii", "abcdefghijklmnop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			limits := sdktrace.NewSpanLimits()
			limits.AttributeValueLengthLimit = 5
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder), sdktrace.WithSpanLimits(limits))
			t.Cleanup(func() {
				if err := provider.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			_, span := provider.Tracer("attribute-limit-regression").Start(t.Context(), "operation")
			span.SetAttributes(attribute.String("value", tc.input))
			span.End()
			ended := recorder.Ended()
			if len(ended) != 1 {
				t.Fatalf("got %d spans, want 1", len(ended))
			}
			attrs := ended[0].Attributes()
			if len(attrs) != 1 {
				t.Fatalf("got %d attributes, want 1", len(attrs))
			}
			want := string([]rune(tc.input)[:5])
			if got := attrs[0].Value.AsString(); got != want {
				t.Fatalf("attribute has %d characters, want the first five", len([]rune(got)))
			}
		})
	}
}
