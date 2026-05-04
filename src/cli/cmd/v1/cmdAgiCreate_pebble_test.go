//go:build !noagi

package cmd

import "testing"

// TestPebbleBudgetInvariants locks down the budget contract for the
// Pebble sizing helpers. These invariants are what the user-visible
// safety story depends on:
//
//  1. Pebble peak (block cache + memtable peak) never exceeds 50% of
//     total host RAM on cloud — the OOM guardrail under sudden plugin
//     query bursts.
//  2. The merged process peak (Pebble + ingest/plugin/Go/Grafana
//     overhead) never exceeds memSize — i.e. it always fits inside
//     the post-OS-reservation budget.
//  3. The memtable stop-writes threshold is at least 8 on cloud — the
//     EFS-jitter buffer floor we want for non-Docker AGI.
//
// If any of these break, downstream behaviour degrades silently
// (OOMs, monitor mis-prediction, ingest stalls), so we lock them
// here.
func TestPebbleBudgetInvariants(t *testing.T) {
	cases := []int64{12, 16, 20, 32, 48, 64, 96, 128, 256}
	for _, totalGiB := range cases {
		totalGiB := totalGiB
		t.Run(formatGiB(totalGiB), func(t *testing.T) {
			totalMem := totalGiB << 30
			memSize := totalMem - agiOSReserveBytes("aws")

			pebbleBudget := computePebbleTotalBudget(totalMem, memSize, "aws")
			cacheBytes := computePebbleCacheBytes(totalMem, memSize, "aws")
			memTableBytes := computePebbleMemTableBytes(totalMem, memSize, "aws")
			threshold := computePebbleStopWritesThreshold(totalMem, memSize, memTableBytes, "aws")

			memtablePeak := uint64(threshold) * memTableBytes
			pebblePeak := cacheBytes + int64(memtablePeak)
			processPeak := pebblePeak + agiNonPebbleOverheadBytes

			// Invariant 1: pebblePeak <= 50% of totalMem.
			if pebblePeak > totalMem/2 {
				t.Errorf("pebblePeak=%d exceeds 50%% of totalMem=%d (over by %d bytes)",
					pebblePeak, totalMem, pebblePeak-totalMem/2)
			}

			// Invariant 2: pebblePeak <= pebbleBudget (helpers
			// must agree with their own budget).
			if pebblePeak > pebbleBudget {
				t.Errorf("pebblePeak=%d exceeds pebbleBudget=%d", pebblePeak, pebbleBudget)
			}

			// Invariant 3: processPeak <= memSize. The merged
			// process must fit within the post-reservation
			// budget so the OS reserve really stays reserved.
			if processPeak > memSize {
				t.Errorf("processPeak=%d (cache=%d + memtable_peak=%d + overhead=%d) exceeds memSize=%d",
					processPeak, cacheBytes, memtablePeak, agiNonPebbleOverheadBytes, memSize)
			}

			// Invariant 4: threshold >= 8 on cloud. Below this
			// the EFS-jitter buffer is too thin and small EFS
			// hiccups stall the writer.
			if threshold < 8 {
				t.Errorf("threshold=%d < 8; EFS-jitter buffer too small", threshold)
			}

			// Invariant 5: threshold <= 32. Recovery from a
			// long EFS hiccup must drain in bounded time.
			if threshold > 32 {
				t.Errorf("threshold=%d > 32; recovery drain unbounded", threshold)
			}

			// Invariant 6: cacheBytes is in [256 MiB, 16 GiB].
			if cacheBytes < int64(256)<<20 {
				t.Errorf("cacheBytes=%d below floor 256 MiB", cacheBytes)
			}
			if cacheBytes > int64(16)<<30 {
				t.Errorf("cacheBytes=%d above ceiling 16 GiB", cacheBytes)
			}

			// Invariant 7: memTableBytes is in [64 MiB, 1 GiB].
			if memTableBytes < uint64(64)<<20 {
				t.Errorf("memTableBytes=%d below floor 64 MiB", memTableBytes)
			}
			if memTableBytes > uint64(1)<<30 {
				t.Errorf("memTableBytes=%d above ceiling 1 GiB", memTableBytes)
			}
		})
	}
}

// TestPebbleBudgetDocker checks the Docker path retains its legacy
// behaviour: cache = memSize/2 (NOT the cloud "half of pebble
// budget" formula, which would halve cache twice on Docker), fixed
// memtable size by host RAM, stop-writes threshold left at the db
// package default (0 = signal). Locks down regressions where the
// cloud refactor accidentally couples Docker to the same budget
// math.
func TestPebbleBudgetDocker(t *testing.T) {
	for _, totalGiB := range []int64{4, 6, 8, 12, 16, 32} {
		totalGiB := totalGiB
		t.Run(formatGiB(totalGiB), func(t *testing.T) {
			totalMem := totalGiB << 30
			memSize := totalMem - agiOSReserveBytes("docker")
			if memSize < 1<<30 {
				t.Skipf("memSize %d below docker minimum", memSize)
			}

			if got, want := computePebbleTotalBudget(totalMem, memSize, "docker"), memSize/2; got != want {
				t.Errorf("docker pebbleBudget = %d, want %d (memSize/2)", got, want)
			}

			// Critical: cache must be memSize/2 (clamped),
			// NOT pebbleBudget/2 = memSize/4. The cloud
			// refactor must not halve Docker cache.
			expectedCache := memSize / 2
			if expectedCache < int64(256)<<20 {
				expectedCache = int64(256) << 20
			}
			if expectedCache > int64(16)<<30 {
				expectedCache = int64(16) << 30
			}
			if got := computePebbleCacheBytes(totalMem, memSize, "docker"); got != expectedCache {
				t.Errorf("docker cacheBytes = %d, want %d (legacy memSize/2 clamped)", got, expectedCache)
			}

			memTableBytes := computePebbleMemTableBytes(totalMem, memSize, "docker")
			if memSize >= int64(6)<<30 {
				if memTableBytes != uint64(256)<<20 {
					t.Errorf("docker memTableBytes (memSize>=6GiB) = %d, want 256 MiB", memTableBytes)
				}
			} else {
				if memTableBytes != uint64(64)<<20 {
					t.Errorf("docker memTableBytes (memSize<6GiB) = %d, want 64 MiB", memTableBytes)
				}
			}

			if got := computePebbleStopWritesThreshold(totalMem, memSize, memTableBytes, "docker"); got != 0 {
				t.Errorf("docker threshold = %d, want 0 (signal db package default)", got)
			}
		})
	}
}

func formatGiB(g int64) string {
	switch g {
	case 4:
		return "4GiB"
	case 6:
		return "6GiB"
	case 8:
		return "8GiB"
	case 12:
		return "12GiB"
	case 16:
		return "16GiB-m7i.xlarge"
	case 20:
		return "20GiB"
	case 32:
		return "32GiB-m7i.2xlarge"
	case 48:
		return "48GiB"
	case 64:
		return "64GiB-r7a.2xlarge"
	case 96:
		return "96GiB"
	case 128:
		return "128GiB"
	case 256:
		return "256GiB"
	}
	return "host"
}
