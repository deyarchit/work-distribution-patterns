// load is a configurable load test tool for the work-distribution-patterns API.
// Run: go run ./tests/load/load.go -url http://localhost:8080
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type submitRequest struct {
	Name       string `json:"name"`
	StageCount int    `json:"stage_count"`
}

type submitResponse struct {
	ID string `json:"id"`
}

type taskResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func submitTask(apiURL string, stages int, submitted, failed *int64, mu *sync.Mutex, taskIDs *[]string) {
	body, marshalErr := json.Marshal(submitRequest{
		Name:       fmt.Sprintf("load-%d", rand.Int63()),
		StageCount: stages,
	})
	if marshalErr != nil {
		atomic.AddInt64(failed, 1)
		return
	}
	resp, err := http.Post(apiURL+"/tasks", "application/json", strings.NewReader(string(body)))
	if err != nil {
		atomic.AddInt64(failed, 1)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		atomic.AddInt64(failed, 1)
		return
	}
	var sr submitResponse
	_ = json.NewDecoder(resp.Body).Decode(&sr)
	atomic.AddInt64(submitted, 1)
	mu.Lock()
	*taskIDs = append(*taskIDs, sr.ID)
	mu.Unlock()
}

func pollUntilDone(apiURL, id string, pollDeadline time.Time, completed *int64) {
	for time.Now().Before(pollDeadline) {
		resp, err := http.Get(fmt.Sprintf("%s/tasks/%s", apiURL, id))
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var tr taskResponse
		_ = json.NewDecoder(resp.Body).Decode(&tr)
		_ = resp.Body.Close()
		if tr.Status == "completed" || tr.Status == "failed" {
			if tr.Status == "completed" {
				atomic.AddInt64(completed, 1)
			}
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func main() {
	url := flag.String("url", "http://localhost:8080", "API base URL")
	rate := flag.Float64("rate", 2.0, "Tasks per second")
	duration := flag.Duration("duration", 30*time.Second, "Test duration")
	stages := flag.Int("stages", 3, "Stages per task")
	flag.Parse()

	log.Printf("Load test: url=%s rate=%.1f/s duration=%s stages=%d",
		*url, *rate, *duration, *stages)

	interval := time.Duration(float64(time.Second) / *rate)
	deadline := time.Now().Add(*duration)

	var (
		submitted int64
		failed    int64
		completed int64
		mu        sync.Mutex
		taskIDs   []string
	)

	// Submission loop
	ticker := time.NewTicker(interval)
	for time.Now().Before(deadline) {
		<-ticker.C
		go submitTask(*url, *stages, &submitted, &failed, &mu, &taskIDs)
	}
	ticker.Stop()

	// Wait up to 120s for tasks to complete (server configures stage duration)
	pollTimeout := 120 * time.Second
	log.Printf("Submitted %d tasks (%d failed submissions). Waiting up to %s for completion...",
		atomic.LoadInt64(&submitted), atomic.LoadInt64(&failed), pollTimeout)

	pollDeadline := time.Now().Add(pollTimeout)
	mu.Lock()
	ids := make([]string, len(taskIDs))
	copy(ids, taskIDs)
	mu.Unlock()

	for _, id := range ids {
		pollUntilDone(*url, id, pollDeadline, &completed)
	}

	total := atomic.LoadInt64(&submitted)
	comp := atomic.LoadInt64(&completed)
	pct := float64(0)
	if total > 0 {
		pct = float64(comp) / float64(total) * 100
	}

	log.Printf("Results: submitted=%d completed=%d failed_submit=%d completion=%.1f%%",
		total, comp, atomic.LoadInt64(&failed), pct)

	if pct < 95.0 {
		log.Fatalf("FAIL: completion %.1f%% < 95%% threshold", pct)
	}
	log.Println("PASS")
}
