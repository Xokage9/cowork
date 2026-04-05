package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"meeting-booking/app/di"
)

type sample struct {
	latency time.Duration
	ok      bool
	code    int
	err     string
}

func main() {
	_ = os.Unsetenv("DATABASE_URL")
	server := httptest.NewServer(di.BuildHTTPHandler())
	defer server.Close()

	adminToken := login(server.URL, "admin")
	userToken := login(server.URL, "user")
	roomID := createRoomAndSchedule(server.URL, adminToken)
	date := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	url := fmt.Sprintf("%s/rooms/%s/slots?date=%s", server.URL, roomID, date)

	workers := 10
	targetRPS := 100
	perWorkerInterval := time.Duration(workers) * time.Second / time.Duration(targetRPS)
	duration := 15 * time.Second
	deadline := time.Now().Add(duration)

	var total uint64
	var success uint64
	statusCounts := map[int]uint64{}
	errorCounts := map[string]uint64{}
	samples := make([]sample, 0, 5000)
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}
			ticker := time.NewTicker(perWorkerInterval)
			defer ticker.Stop()
			for time.Now().Before(deadline) {
				<-ticker.C
				start := time.Now()
				req, _ := http.NewRequest(http.MethodGet, url, nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				resp, err := client.Do(req)
				lat := time.Since(start)
				atomic.AddUint64(&total, 1)
				code := 0
				if resp != nil {
					code = resp.StatusCode
				}
				ok := err == nil && code == http.StatusOK
				if ok {
					atomic.AddUint64(&success, 1)
				}
				if resp != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
				mu.Lock()
				statusCounts[code]++
				if err != nil {
					errorCounts[err.Error()]++
				}
				errText := ""
				if err != nil {
					errText = err.Error()
				}
				samples = append(samples, sample{latency: lat, ok: ok, code: code, err: errText})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	latencies := make([]time.Duration, 0, len(samples))
	for _, s := range samples {
		if s.ok {
			latencies = append(latencies, s.latency)
		}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	p50 := percentile(latencies, 0.50)
	p95 := percentile(latencies, 0.95)
	p99 := percentile(latencies, 0.99)
	rps := float64(total) / duration.Seconds()
	successRate := 0.0
	if total > 0 {
		successRate = float64(success) * 100 / float64(total)
	}

	fmt.Printf("workers=%d target_rps=%d duration=%s total=%d success=%d success_rate=%.2f%% rps=%.2f p50=%s p95=%s p99=%s\n",
		workers, targetRPS, duration.String(), total, success, successRate, rps, p50.String(), p95.String(), p99.String())
	fmt.Printf("status_counts=%v\n", statusCounts)
	if len(errorCounts) > 0 {
		fmt.Printf("error_counts=%v\n", errorCounts)
	}
}

func percentile(latencies []time.Duration, p float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	idx := int(float64(len(latencies)-1) * p)
	return latencies[idx]
}

func login(baseURL, role string) string {
	resp := request(http.MethodPost, baseURL+"/dummyLogin", "", map[string]any{"role": role})
	return resp["accessToken"].(string)
}

func createRoomAndSchedule(baseURL, adminToken string) string {
	room := request(http.MethodPost, baseURL+"/rooms", adminToken, map[string]any{"name": "Load Test Room", "capacity": 10})
	roomID := room["id"].(string)
	_ = request(http.MethodPost, baseURL+"/rooms/"+roomID+"/schedule", adminToken, map[string]any{
		"weekdays": []int{0, 1, 2, 3, 4, 5, 6}, "startMinutes": 540, "endMinutes": 1080,
	})
	return roomID
}

func request(method, url, token string, payload any) map[string]any {
	var body io.Reader
	if payload != nil {
		buf, _ := json.Marshal(payload)
		body = bytes.NewBuffer(buf)
	}
	req, _ := http.NewRequest(method, url, body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	parsed := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return parsed
}

