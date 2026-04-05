package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/simonwhitaker/geo-energy-datadog/energy"
)

type ReadingMode int

const (
	LIVE ReadingMode = 1 << iota
	PERIODIC
)

const (
	healthServerAddr         = ":8080"
	liveReadinessTimeout     = time.Minute
	periodicReadinessTimeout = 15 * time.Minute
)

type healthState struct {
	ready          atomic.Bool
	lastLiveOK     atomic.Int64
	lastPeriodicOK atomic.Int64
}

func (h *healthState) markSuccess(mode ReadingMode) {
	now := time.Now().Unix()

	if mode&LIVE != 0 {
		h.lastLiveOK.Store(now)
	}
	if mode&PERIODIC != 0 {
		h.lastPeriodicOK.Store(now)
	}

	h.ready.Store(true)
}

func (h *healthState) livenessHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *healthState) readinessHandler(w http.ResponseWriter, _ *http.Request) {
	if !h.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}

	now := time.Now()
	lastLiveOK := time.Unix(h.lastLiveOK.Load(), 0)
	lastPeriodicOK := time.Unix(h.lastPeriodicOK.Load(), 0)

	if now.Sub(lastLiveOK) > liveReadinessTimeout {
		http.Error(w, "live readings stale", http.StatusServiceUnavailable)
		return
	}
	if now.Sub(lastPeriodicOK) > periodicReadinessTimeout {
		http.Error(w, "periodic readings stale", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func startHealthServer(logger *log.Logger, health *healthState) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health.livenessHandler)
	mux.HandleFunc("/readyz", health.readinessHandler)

	server := &http.Server{
		Addr:    healthServerAddr,
		Handler: mux,
	}

	go func() {
		logger.Printf("Health server listening on %s", healthServerAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Health server failed: %v", err)
		}
	}()

	return server
}

func getMeterData(reader energy.EnergyDataReader, writers []energy.EnergyDataWriter, mode ReadingMode) error {
	allReadings := []energy.Reading{}

	if mode&PERIODIC != 0 {
		// Get periodic meter data
		readings, err := reader.GetMeterReadings()
		if err != nil {
			return err
		}
		allReadings = append(allReadings, readings...)
	}
	if mode&LIVE != 0 {
		// Get live meter data
		readings, err := reader.GetLiveReadings()
		if err != nil {
			return err
		}

		allReadings = append(allReadings, readings...)
	}

	for _, w := range writers {
		err := w.WriteReadings(allReadings)
		if err != nil {
			return err
		}
	}
	return nil
}

// waitForConnection retries getMeterData with exponential backoff until it succeeds
func waitForConnection(logger *log.Logger, reader energy.EnergyDataReader, writers []energy.EnergyDataWriter) {
	backoff := time.Second * 5
	maxBackoff := time.Minute * 2

	for {
		err := getMeterData(reader, writers, LIVE|PERIODIC)
		if err == nil {
			logger.Println("Connected successfully")
			return
		}

		logger.Printf("Connection failed: %v (retrying in %v)", err, backoff)
		time.Sleep(backoff)

		// Exponential backoff with cap
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	health := &healthState{}

	// Configure reader
	geoUsername := os.Getenv("GEO_USERNAME")
	geoPassword := os.Getenv("GEO_PASSWORD")
	reader := energy.NewGeoEnergyDataReader(geoUsername, geoPassword)

	// Configure writers
	writers := []energy.EnergyDataWriter{
		energy.NewLoggerWriter(logger),
	}

	if _, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT"); ok {
		otelHostname := getEnvOrDefault("OTEL_HOSTNAME", "localhost")
		otelWriter, err := energy.NewOTelWriter(context.Background(), otelHostname, logger)
		if err != nil {
			logger.Fatalf("Failed to initialize OTel writer: %v", err)
		}
		writers = append(writers, otelWriter)
	} else {
		logger.Println("Skipping OTel; OTEL_EXPORTER_OTLP_ENDPOINT not set")
	}

	healthServer := startHealthServer(logger, health)

	// Wait for initial connection with retry
	waitForConnection(logger, reader, writers)
	health.markSuccess(LIVE | PERIODIC)

	tickLive := time.NewTicker(time.Second * time.Duration(10))
	tickPeriodic := time.NewTicker(time.Second * time.Duration(300))

	go func() {
		for {
			select {
			case <-tickLive.C:
				if err := getMeterData(reader, writers, LIVE); err != nil {
					logger.Printf("Error getting live data: %v", err)
				} else {
					health.markSuccess(LIVE)
				}
			case <-tickPeriodic.C:
				if err := getMeterData(reader, writers, PERIODIC); err != nil {
					logger.Printf("Error getting periodic data: %v", err)
				} else {
					health.markSuccess(PERIODIC)
				}
			}
		}
	}()

	// Wait for a SIGINT or SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	logger.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Printf("Error shutting down health server: %v", err)
	}
	for _, w := range writers {
		if err := w.Close(); err != nil {
			logger.Printf("Error closing writer: %v", err)
		}
	}
}
