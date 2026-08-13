package executor

import (
	"sync"
	"testing"
	"time"
)

// TestConnectorRegistry_RaceFree exercises the concurrent access pattern
// introduced by live PTY resize / output streaming: PlaybookExecutor
// initConnectors / closeConnectors mutate the registry while the
// resize-forwarding goroutine concurrently reads it via Snapshot.
//
// Pre-registry, the bare map version of this test would fail under
// `go test -race` with a "concurrent map read and map write" panic.
// The registry's RWMutex makes the same access pattern safe.
func TestConnectorRegistry_RaceFree(t *testing.T) {
	r := newConnectorRegistry()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writers: simulate init/close churn.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		host := "host"
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				r.Put(host, nil)
				_ = r.Has(host)
				r.Delete(host)
			}
		}(i)
	}

	// Readers: simulate Resize/SetLiveOutput snapshots.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = r.Snapshot()
				_ = r.Get("host")
			}
		}()
	}

	// Run the storm for 50ms then signal everyone to stop.
	doneTimer := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(stop)
		close(doneTimer)
	}()
	<-doneTimer
	wg.Wait()
}
