package compute

import (
	"strings"
	"testing"

	agenttypes "vraxel.io/vraxel/lib/agent/types"
)

// The two metric backends promise to answer identically for the same
// window: an operator moving a host from the lite tier to the full one
// must not see charts appear, vanish or change shape. Nothing enforced
// that promise, and adding twenty-nine charts to the agent's ring while
// leaving the VictoriaMetrics table alone produced exactly the failure it
// warns about -- every new chart empty on the tier this deployment runs.
//
// These tests are the enforcement. They compare the two vocabularies as
// sets, so the next series added to either side fails here rather than in
// somebody's browser.

// allChartSeries is every name the agent's ring can derive, which is the
// vocabulary the frontend is written against.
func allChartSeries() []string {
	return []string{
		agenttypes.SeriesCPUUsedPct, agenttypes.SeriesCPUModePct,
		agenttypes.SeriesMemUsedPct, agenttypes.SeriesMemUsed, agenttypes.SeriesSwapUsed,
		agenttypes.SeriesLoad1, agenttypes.SeriesLoad5, agenttypes.SeriesLoad15,
		agenttypes.SeriesFSUsedPct, agenttypes.SeriesFSSize, agenttypes.SeriesFSInodesPct,
		agenttypes.SeriesDiskReadBps, agenttypes.SeriesDiskWriteBps, agenttypes.SeriesDiskUtilPct,
		agenttypes.SeriesDiskReadIOPS, agenttypes.SeriesDiskWriteIOPS,
		agenttypes.SeriesDiskReadWait, agenttypes.SeriesDiskWriteWait,
		agenttypes.SeriesNetRxBps, agenttypes.SeriesNetTxBps,
		agenttypes.SeriesNetRxPps, agenttypes.SeriesNetTxPps,
		agenttypes.SeriesNetRxErrs, agenttypes.SeriesNetTxErrs,
		agenttypes.SeriesNetRxDrops, agenttypes.SeriesNetTxDrops,
		agenttypes.SeriesPSICPUPct, agenttypes.SeriesPSIMemPct, agenttypes.SeriesPSIIOPct,
		agenttypes.SeriesMemFree,
		agenttypes.SeriesMemBuffers, agenttypes.SeriesMemCached,
		agenttypes.SeriesSwapInPps, agenttypes.SeriesSwapOutPps,
		agenttypes.SeriesPageFaults, agenttypes.SeriesOOMKills,
		agenttypes.SeriesCtxSwitches, agenttypes.SeriesInterrupts,
		agenttypes.SeriesTCPRetrans, agenttypes.SeriesTCPInUse,
		agenttypes.SeriesSocketsUsed, agenttypes.SeriesConntrackPct,
		agenttypes.SeriesUptimeSec, agenttypes.SeriesProcsRunning, agenttypes.SeriesProcsBlocked,
		agenttypes.SeriesFDUsedPct, agenttypes.SeriesTimeDriftSec,
		agenttypes.SeriesTempCelsius,
	}
}

func TestVMVocabularyCoversEveryChartSeries(t *testing.T) {
	for _, name := range allChartSeries() {
		if _, ok := vmVocabulary[name]; !ok {
			t.Errorf("%s has no PromQL: it works on the agent tier and is empty on the full one", name)
		}
	}
}

// An entry missing from the order list is never queried when the caller
// asks for everything, which is the frontend's normal request -- so the
// chart is silently empty with the translation sitting right there.
func TestVMVocabularyOrderIsComplete(t *testing.T) {
	listed := map[string]bool{}
	for _, n := range vmVocabularyOrder {
		if listed[n] {
			t.Errorf("%s listed twice in vmVocabularyOrder", n)
		}
		listed[n] = true
		if _, ok := vmVocabulary[n]; !ok {
			t.Errorf("%s is ordered but has no expression", n)
		}
	}
	for name := range vmVocabulary {
		if !listed[name] {
			t.Errorf("%s has an expression but is never queried: add it to vmVocabularyOrder", name)
		}
	}
}

// Nothing is queried from VictoriaMetrics that the agent tier cannot
// also produce, and no two chart names collide.
//
// This does NOT prove the ring derives each one -- a name the ring does
// not know is silently ignored rather than erroring, so that has to be
// checked in nodemetrics' own tests, where it is.
func TestChartSeriesNamesAreDistinctAndQueryable(t *testing.T) {
	known := map[string]bool{}
	for _, n := range allChartSeries() {
		known[n] = true
	}
	for _, n := range vmVocabularyOrder {
		if !known[n] {
			t.Errorf("%s is queried from VictoriaMetrics but is not a chart series the agent tier derives", n)
		}
	}
	// Guard against the constants themselves drifting into an empty or
	// duplicated name, which would make both tiers agree on nothing.
	seen := map[string]bool{}
	for _, n := range allChartSeries() {
		if n == "" {
			t.Fatal("a chart series constant is empty")
		}
		if seen[n] {
			t.Errorf("duplicate chart series name %q", n)
		}
		seen[n] = true
	}
}

// The PromQL filters are generated from the same lists the agent's
// predicates use, so a type added to one is in the other. This checks the
// generated matcher actually mentions them rather than, say, silently
// producing an empty alternation that matches everything.
func TestPromFiltersMirrorThePredicates(t *testing.T) {
	for _, fs := range []string{"tmpfs", "nfs4", "cifs", "overlay"} {
		if !strings.Contains(fsFilter, fs) {
			t.Errorf("fsFilter does not exclude %q: %s", fs, fsFilter)
		}
		if agenttypes.RealFSType(fs) {
			t.Errorf("RealFSType(%q) disagrees with the filter", fs)
		}
	}
	if strings.Contains(fsFilter, `"ext4`) || strings.Contains(fsFilter, `|ext4`) {
		t.Errorf("fsFilter must not exclude real storage: %s", fsFilter)
	}
	for _, dev := range []string{"lo", "veth", "docker", "br-"} {
		if !strings.Contains(netFilter, dev) {
			t.Errorf("netFilter does not exclude %q: %s", dev, netFilter)
		}
	}
	// A prefix rule needs its wildcard: PromQL's =~ is whole-label, so a
	// bare "veth" would match only an interface literally named veth.
	if !strings.Contains(netFilter, "veth.*") {
		t.Errorf("netFilter prefixes must end in .*: %s", netFilter)
	}
}

// Percentages are clamped on both tiers, or a deep disk queue reads
// 100.4% busy on one and 100% on the other.
func TestVMPercentagesAreClamped(t *testing.T) {
	pctSeries := []string{
		agenttypes.SeriesCPUUsedPct, agenttypes.SeriesCPUModePct,
		agenttypes.SeriesMemUsedPct, agenttypes.SeriesSwapUsed,
		agenttypes.SeriesFSUsedPct, agenttypes.SeriesFSInodesPct,
		agenttypes.SeriesDiskUtilPct,
		agenttypes.SeriesPSICPUPct, agenttypes.SeriesPSIMemPct, agenttypes.SeriesPSIIOPct,
		agenttypes.SeriesConntrackPct, agenttypes.SeriesFDUsedPct,
	}
	for _, name := range pctSeries {
		v, ok := vmVocabulary[name]
		if !ok {
			t.Fatalf("%s missing", name)
		}
		if !strings.HasPrefix(v.expr, "clamp(") {
			t.Errorf("%s is a percentage and is not clamped: %s", name, v.expr)
		}
	}
}
