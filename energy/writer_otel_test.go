package energy

import (
	"context"
	"log"
	"os"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestWriteReadings(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	logger := log.New(os.Stdout, "", log.LstdFlags)

	writer, err := newOTelWriter(context.Background(), reader, "test-host", logger)
	if err != nil {
		t.Fatalf("Failed to create OTelWriter: %v", err)
	}
	defer writer.Close()

	readings := []Reading{
		{Commodity: ELECTRICITY, ReadingType: LIVE, Value: 42.5},
		{Commodity: GAS, ReadingType: LIVE, Value: 10.0},
		{Commodity: ELECTRICITY, ReadingType: METER, Value: 1234.56},
		{Commodity: GAS, ReadingType: METER, Value: 789.0},
	}

	if err := writer.WriteReadings(readings); err != nil {
		t.Fatalf("WriteReadings failed: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	expected := map[string]float64{
		"energy.live.electricity":  42.5,
		"energy.live.gas":          10.0,
		"energy.meter.electricity": 1234.56,
		"energy.meter.gas":         789.0,
	}

	found := make(map[string]float64)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			gauge, ok := m.Data.(metricdata.Gauge[float64])
			if !ok {
				t.Fatalf("Metric %s is not a float64 gauge", m.Name)
			}
			if len(gauge.DataPoints) != 1 {
				t.Fatalf("Expected 1 data point for %s, got %d", m.Name, len(gauge.DataPoints))
			}
			found[m.Name] = gauge.DataPoints[0].Value
		}
	}

	for name, expectedVal := range expected {
		actual, ok := found[name]
		if !ok {
			t.Errorf("Missing metric: %s", name)
			continue
		}
		if actual != expectedVal {
			t.Errorf("Metric %s: expected %v, got %v", name, expectedVal, actual)
		}
	}
}
