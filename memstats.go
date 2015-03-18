package modern

import (
	js "github.com/rshmelev/go-json-light"
	"runtime"
	"sync"
	"time"
)

var memtracking bool
var memtrackingMutex sync.RWMutex
var someMemStats js.IObject

func init() {
	someMemStats = js.NewObject()
}

func GetSomeMemStats() js.IObject {
	memtrackingMutex.RLock()
	defer memtrackingMutex.RUnlock()
	return someMemStats
}

func TrackMemStats() {
	sleepTime := time.Second
	if memtracking {
		return
	}
	memtracking = true
	go func() {
		memStats := &runtime.MemStats{}
		lastSampleTime := time.Now()
		var lastPauseNs uint64 = 0
		var lastNumGc uint32 = 0

		nsInMs := float64(time.Millisecond)

		for {
			runtime.ReadMemStats(memStats)
			now := time.Now()
			s := js.NewObject()

			s.Put("goroutines", (runtime.NumGoroutine()))
			s.Put("memory.heap.objects", (memStats.HeapObjects))
			s.Put("memory.allocated", (memStats.Alloc))
			s.Put("memory.mallocs", (memStats.Mallocs))
			s.Put("memory.frees", (memStats.Frees))
			s.Put("memory.gc.total_pause", float64(memStats.PauseTotalNs)/nsInMs)
			s.Put("memory.heap", (memStats.HeapAlloc))
			s.Put("memory.sys", (memStats.Sys))
			s.Put("memory.stack", (memStats.StackInuse))

			if lastPauseNs > 0 {
				pauseSinceLastSample := memStats.PauseTotalNs - lastPauseNs
				s.Put("memory.gc.pause_per_second", float64(pauseSinceLastSample)/nsInMs/sleepTime.Seconds())
			}
			lastPauseNs = memStats.PauseTotalNs

			countGc := int(memStats.NumGC - lastNumGc)
			if lastNumGc > 0 {
				diff := float64(countGc)
				diffTime := now.Sub(lastSampleTime).Seconds()
				s.Put("memory.gc.gc_per_second", diff/diffTime)
			}

			// keep track of the previous state
			lastNumGc = memStats.NumGC
			lastSampleTime = now

			memtrackingMutex.Lock()
			someMemStats = s
			memtrackingMutex.Unlock()

			time.Sleep(sleepTime)
		}
	}()
}
