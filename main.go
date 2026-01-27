package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/simonwhitaker/geo-energy-datadog/energy"
)

type ReadingMode int

const (
	LIVE ReadingMode = 1 << iota
	PERIODIC
)

func getMeterData(logger *log.Logger, reader energy.EnergyDataReader, writers []energy.EnergyDataWriter, mode ReadingMode) error {
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
		err := getMeterData(logger, reader, writers, LIVE|PERIODIC)
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

	// Configure reader
	geoUsername := os.Getenv("GEO_USERNAME")
	geoPassword := os.Getenv("GEO_PASSWORD")
	reader := energy.NewGeoEnergyDataReader(geoUsername, geoPassword)

	// Configure writers
	writers := []energy.EnergyDataWriter{
		energy.NewLoggerWriter(logger),
	}

	if _, ok := os.LookupEnv("DD_API_KEY"); ok {
		datadogHostname := getEnvOrDefault("DD_HOSTNAME", "localhost")
		writers = append(writers, energy.NewDatadogWriter(datadogHostname, logger))
	} else {
		logger.Println("Skipping Datadog; DD_API_KEY not set")
	}

	// Wait for initial connection with retry
	waitForConnection(logger, reader, writers)

	tickLive := time.NewTicker(time.Second * time.Duration(10))
	tickPeriodic := time.NewTicker(time.Second * time.Duration(300))

	go func() {
		for {
			select {
			case <-tickLive.C:
				if err := getMeterData(logger, reader, writers, LIVE); err != nil {
					logger.Printf("Error getting live data: %v", err)
				}
			case <-tickPeriodic.C:
				if err := getMeterData(logger, reader, writers, PERIODIC); err != nil {
					logger.Printf("Error getting periodic data: %v", err)
				}
			}
		}
	}()

	// Wait for a SIGINT or SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
}
