package appmetrics

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"vraxel.io/vraxel/lib/buildinfo"
	"vraxel.io/vraxel/lib/cgroup"
	"vraxel.io/vraxel/lib/lflag"
	"vraxel.io/vraxel/lib/memory"
	"vraxel.io/vraxel/lib/utils/bytesutil"
)

var exposeMetadata = flag.Bool("metrics.exposeMetadata", false, "Whether to expose TYPE and HELP metadata at the /metrics page, which is exposed at -httpListenAddr . "+
	"The metadata may be needed when the /metrics page is consumed by systems, which require this information. For example, Managed Prometheus in Google Cloud - "+
	"https://cloud.google.com/stackdriver/docs/managed-prometheus/troubleshooting#missing-metric-type")

var exposeMetadataOnce sync.Once

func initExposeMetadata() {
	metrics.ExposeMetadata(*exposeMetadata)
}

// WritePrometheusMetrics writes all the registered metrics to w in Prometheus exposition format.
func WritePrometheusMetrics(w io.Writer) {
	exposeMetadataOnce.Do(initExposeMetadata)

	currentTime := time.Now()
	metricsCacheLock.Lock()
	if currentTime.Sub(metricsCacheLastUpdateTime) > time.Second {
		var bb bytesutil.ByteBuffer
		writePrometheusMetrics(&bb)
		metricsCache.Store(&bb)
		metricsCacheLastUpdateTime = currentTime
	}
	metricsCacheLock.Unlock()

	bb := metricsCache.Load()
	_, _ = w.Write(bb.B)
}

var (
	metricsCacheLock           sync.Mutex
	metricsCacheLastUpdateTime time.Time
	metricsCache               atomic.Pointer[bytesutil.ByteBuffer]
)

func writePrometheusMetrics(w io.Writer) {
	metrics.WritePrometheus(w, true)
	metrics.WriteFDMetrics(w)

	metrics.WriteGaugeUint64(w, fmt.Sprintf("vraxel_app_version{version=%q, short_version=%q}", buildinfo.Version, buildinfo.ShortVersion()), 1)
	metrics.WriteGaugeUint64(w, "vraxel_allowed_memory_bytes", uint64(memory.Allowed()))
	metrics.WriteGaugeUint64(w, "vraxel_available_memory_bytes", uint64(memory.Allowed()+memory.Remaining()))
	metrics.WriteGaugeUint64(w, "vraxel_available_cpu_cores", uint64(cgroup.AvailableCPUs()))
	metrics.WriteGaugeUint64(w, "vraxel_gogc", uint64(cgroup.GetGOGC()))

	// Export start time and uptime in seconds
	metrics.WriteGaugeUint64(w, "vraxel_app_start_timestamp", uint64(startTime.Unix()))
	metrics.WriteGaugeUint64(w, "vraxel_app_uptime_seconds", uint64(time.Since(startTime).Seconds()))

	// Export flags as metrics.
	isSetMap := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		isSetMap[f.Name] = true
	})
	metrics.WriteMetadataIfNeeded(w, "flag", "gauge")
	flag.VisitAll(func(f *flag.Flag) {
		lname := strings.ToLower(f.Name)
		value := f.Value.String()
		if lflag.IsSecretFlag(lname) {
			// Do not expose passwords and keys to prometheus.
			value = "secret"
		}
		isSet := "false"
		if isSetMap[f.Name] {
			isSet = "true"
		}
		_, _ = fmt.Fprintf(w, "flag{name=%q, value=%q, is_set=%q} 1\n", f.Name, value, isSet)
	})
}

var startTime = time.Now()
