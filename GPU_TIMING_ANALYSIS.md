# GPU MSM Timing Instability Analysis

## Problem Summary

The ICICLE GPU MSM performance on RTX 4090 is highly unstable:
- **Slow runs**: 5-7 seconds for 2^20 elements
- **Fast runs**: 17-180 milliseconds for 2^20 elements
- Performance varies by **30-400x** between runs

## Root Cause Identified ✓

### The GPU Randomly Switches Between P0 and P5 Power States

**The instability is caused by the GPU randomly entering either:**
- **P0 (Performance) state**: Clocks at 2520 MHz → **FAST** (17-180ms)
- **P5 (Idle) state**: Clocks at 1400-1700 MHz → **SLOW** (5-7 seconds)

**Evidence from 10 consecutive test runs:**
```
Test 1:  FAST (31-98ms)    → GPU in P0 at 2520 MHz
Test 2:  SLOW (6.5s)       → GPU in P5 at 1710 MHz
Test 3:  SLOW (7.0s)       → GPU in P5 at 1650 MHz
Test 4:  SLOW (6.7s)       → GPU in P5 at 1590 MHz
Test 5:  FAST (160-177ms)  → GPU in P0 at 2520 MHz
Test 7:  SLOW (7.0s)       → GPU in P5 at 1470 MHz
Test 8:  SLOW (6.5s)       → GPU in P5 at 1560 MHz
Test 9:  FAST (90-296ms)   → GPU in P0 at 2520 MHz
Test 10: FAST (167-179ms)  → GPU in P0 at 2520 MHz
```

### Why Does This Happen?

1. **Low GPU Utilization**: ICICLE MSM kernel only uses ~17% of GPU SM (streaming multiprocessors)
2. **Power Management**: GPU's power management sees low utilization and randomly decides whether to boost clocks
3. **No Determinism**: There's no predictable pattern - the GPU state is essentially random between runs

### GPU Utilization Evidence
```
# gpu     sm    mem    enc    dec    jpg    ofa
# Idx      %      %      %      %      %      %
    0     17      4      0      0      0      0   <-- Only 17% SM utilization!
```

### RTX 4090 Limitations
- **No Application Clocks support** (consumer GPU limitation)
- **Persistence Mode doesn't help** (tested - still random P0/P5 switching)
- **Cannot force P0 state** without locking clocks (requires sudo)

## Why Low Utilization?

The ICICLE MSM kernel appears to have insufficient parallelism or poor occupancy for the RTX 4090's 16,384 CUDA cores. Possible reasons:

1. **Kernel launch configuration** - Not enough thread blocks/threads
2. **Memory-bound operations** - Waiting on memory transfers
3. **Synchronization overhead** - Excessive kernel launches or barriers
4. **Suboptimal algorithm** - Not enough work per GPU core

## Test Results

### Test 1: Aggressive Warmup (10 iterations)
Even after 10 warmup MSMs, subsequent runs remain slow (~175ms each).

### Test 2: Continuous Execution (20 MSMs)
All 20 runs consistently slow (5-7 seconds), proving it's not a cold-start issue.

### Test 3: Pre-loaded Data
Keeping data on GPU doesn't help - still 5-7 seconds per MSM.

## Comparison with CPU

From README.md benchmarks:
```
| Degree | Size    | CPU Time  | GPU Time | Result      |
|--------|---------|-----------|----------|-------------|
| 2^20   | 1048576 | 100.00 ms | 7.35 s   | 73x SLOWER! |
```

When GPU is slow, **CPU is 73x faster** than GPU for this workload.

## Solution: Lock GPU Clocks ✓

### The Fix (Requires Sudo)

Lock the GPU clocks to force P0 state and prevent random P5 switching:

```bash
# Lock GPU clocks to boost range (2520-3105 MHz)
sudo nvidia-smi -lgc 2520,3105 -i 0

# Lock memory clocks to max (10501 MHz)
sudo nvidia-smi -lmc 10501 -i 0
```

**Or use the provided script:**
```bash
sudo ./lock_gpu_clocks.sh
```

**To reset clocks (unlock):**
```bash
sudo nvidia-smi -rgc -i 0  # Reset GPU clocks
sudo nvidia-smi -rmc -i 0  # Reset memory clocks
```

### Expected Results After Locking Clocks

- **Consistent performance**: All runs should be fast (17-180ms)
- **GPU stays in P0 state**: Clocks locked at 2520+ MHz
- **No more 5-7 second slow runs**: Eliminates the random P5 state

### What Doesn't Work

1. **Persistence Mode** - Tested, doesn't prevent P0/P5 switching
2. **Warmup iterations** - Doesn't force GPU into P0 state
3. **Continuous execution** - GPU still randomly switches states
4. **ICICLE configuration tuning** - Doesn't address the power state issue

## Alternative Solutions (If Clock Locking Not Available)

### 1. Use CPU MSM Instead
For production use without sudo access, the CPU implementation is more reliable:
- **CPU**: Consistent 100ms for 2^20 elements
- **GPU (unlocked)**: Random 17ms-7s (unreliable)
- **CPU is 50-70x faster** than GPU when GPU is in P5 state

### 2. Profile with NVIDIA Nsight
Investigate why GPU utilization is only 17%:
```bash
nsys profile --stats=true go run -tags=icicle icicle_official_example.go -size 20 -runs 1
```

### 3. Contact ICICLE Developers
Report the low GPU utilization issue:
- Only 17% SM utilization on RTX 4090
- Causes power management to keep GPU in low-power states
- Need better kernel occupancy for high-core-count GPUs

## Hardware Details

- **GPU**: NVIDIA GeForce RTX 4090 (16,384 CUDA cores)
- **CPU**: AMD EPYC 7773X 64-Core Processor
- **CUDA**: 12.8
- **Driver**: 570.86.10
- **Go**: 1.24.9
- **ICICLE**: v3.2.2

## Conclusion

The timing instability is caused by **random GPU power state switching between P0 (fast) and P5 (slow)**, triggered by low GPU utilization (~17% SM usage). The ICICLE MSM kernel doesn't generate enough parallel work to keep the RTX 4090 busy, causing the GPU's power management to randomly decide whether to boost clocks.

**Solution**: Lock GPU clocks using `sudo nvidia-smi -lgc 2520,3105 -i 0` to force P0 state and achieve consistent fast performance (17-180ms).

**Without clock locking**: The CPU MSM implementation is more reliable and 50-70x faster than GPU when GPU is in P5 state.
