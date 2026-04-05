package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReadinessHandlerReturnsUnavailableBeforeInitialSuccess(t *testing.T) {
	health := &healthState{}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.readinessHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestReadinessHandlerReturnsOKAfterRecentSuccess(t *testing.T) {
	health := &healthState{}
	health.markSuccess(LIVE | PERIODIC)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.readinessHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestReadinessHandlerReturnsUnavailableWhenLiveReadingsAreStale(t *testing.T) {
	now := time.Now()
	health := &healthState{}
	health.ready.Store(true)
	health.lastLiveOK.Store(now.Add(-liveReadinessTimeout - time.Second).Unix())
	health.lastPeriodicOK.Store(now.Unix())
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.readinessHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestReadinessHandlerReturnsUnavailableWhenPeriodicReadingsAreStale(t *testing.T) {
	now := time.Now()
	health := &healthState{}
	health.ready.Store(true)
	health.lastLiveOK.Store(now.Unix())
	health.lastPeriodicOK.Store(now.Add(-periodicReadinessTimeout - time.Second).Unix())
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	health.readinessHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}
