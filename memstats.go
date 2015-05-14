package modern

import (
	"runtime"
	"sync"
	"time"

	js "github.com/rshmelev/go-json-light"
)

var memtracking bool
var memtrackingMutex sync.RWMutex
var someMemStats js.IObject

func init() {
	someMemStats = js.NewEmptyObject()
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
			s := js.NewObjectFromMap(map[string]interface{}{
				"goroutines":            runtime.NumGoroutine(),
				"memory.heap.objects":   memStats.HeapObjects,
				"memory.allocated":      memStats.Alloc,
				"memory.mallocs":        memStats.Mallocs,
				"memory.frees":          memStats.Frees,
				"memory.gc.total_pause": float64(memStats.PauseTotalNs) / nsInMs,
				"memory.heap":           memStats.HeapAlloc,
				"memory.sys":            memStats.Sys,
				"memory.stack":          memStats.StackInuse,
			})

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
