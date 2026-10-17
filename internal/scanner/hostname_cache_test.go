package scanner

import (
	"sync"
	"testing"
	"time"
)

func TestHostnameCacheDeduplicatesConcurrentLookups(t *testing.T) {
	cache := newHostnameCache(time.Minute)
	var calls int
	var callsMu sync.Mutex
	lookup := func() string {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return "OFFICE-PC"
	}

	const requests = 16
	results := make(chan string, requests)
	var wait sync.WaitGroup
	for index := 0; index < requests; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- cache.resolve("192.168.1.10", lookup)
		}()
	}
	wait.Wait()
	close(results)

	for hostname := range results {
		if hostname != "OFFICE-PC" {
			t.Fatalf("hostname = %q, want OFFICE-PC", hostname)
		}
	}
	if calls != 1 {
		t.Fatalf("lookup calls = %d, want 1", calls)
	}
}

func TestHostnameCacheRetriesEmptyResults(t *testing.T) {
	cache := newHostnameCache(time.Minute)
	calls := 0
	lookup := func() string {
		calls++
		return ""
	}

	if hostname := cache.resolve("192.168.1.20", lookup); hostname != "" {
		t.Fatalf("first hostname = %q, want empty", hostname)
	}
	if hostname := cache.resolve("192.168.1.20", lookup); hostname != "" {
		t.Fatalf("second hostname = %q, want empty", hostname)
	}
	if calls != 2 {
		t.Fatalf("lookup calls = %d, want 2", calls)
	}
}
