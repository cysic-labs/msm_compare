# ICICLE MSM Benchmark

Benchmarking tool comparing ICICLE v2 vs v3 GPU MSM performance on the BN254 elliptic curve.

## Key Finding: ICICLE v3 has severe timing instability

For $2^{20}$ elements on RTX 4090:

| Version | Typical MSM | Worst Case | Variation |
|---------|-------------|------------|-----------|
| **ICICLE v2** | 17-20ms | ~160ms | ~8x |
| **ICICLE v3** | 20ms | 6-9 seconds | 300-400x |

**Recommendation: Use ICICLE v2** for stable, predictable performance.

![msm_time](./pic/msm_time.png)

## ICICLE v3 Problem

The v3 GPU MSM time is highly unstable due to low GPU utilization (~17% SM usage) causing random power state switching:
- **P0 state (2520 MHz)**: Fast runs (~20ms)
- **P5 state (1400-1700 MHz)**: Slow runs (6-9 seconds)

See `GPU_TIMING_ANALYSIS.md` for detailed investigation.

## Quick Start

### ICICLE v2 (Recommended)

```bash
make icicle-v2-large  # Run v2 MSM benchmark (2^20 elements)
```

### ICICLE v3

```bash
make lib              # Download v3 libraries (first time only)
make icicle-large     # Run v3 MSM benchmark (2^20 elements)
```

## Benchmark Results

### ICICLE v2 (Stable)

```
MSM size: 2^20 = 1048576 elements
  Run 1/5: Total: 25.6ms (Copy: 6.1ms, MSM: 17.9ms)
  Run 2/5: Total: 25.8ms (Copy: 6.4ms, MSM: 18.0ms)
  Run 3/5: Total: 25.8ms (Copy: 6.4ms, MSM: 17.9ms)
  Run 4/5: Total: 104.6ms (Copy: 6.7ms, MSM: 27.6ms)
  Run 5/5: Total: 100.4ms (Copy: 82.1ms, MSM: 16.9ms)

Average MSM time: ~19ms
```

### ICICLE v3 (Unstable)

```
MSM size: 2^20 = 1048576 elements
  Run 1/3: Total: 9.35s (Copy: 77ms, MSM: 9.27s)   <- SLOW
  Run 2/3: Total: 7.09s (Copy: 92ms, MSM: 7.00s)   <- SLOW
  Run 3/3: Total: 7.22s (Copy: 111ms, MSM: 7.10s)  <- SLOW
```

Sometimes v3 is fast (~20ms), but it's unpredictable.

## CPU vs GPU (v3)

When v3 GPU is slow, CPU is significantly faster:

| Degree | Size    | CPU Time  | GPU Time | Result |
|--------|---------|-----------|----------|--------|
| 2^18   | 262144  | 29.00 ms  | 2.79 s   | 93x CPU faster |
| 2^19   | 524288  | 66.00 ms  | 3.83 s   | 57x CPU faster |
| 2^20   | 1048576 | 100.00 ms | 7.35 s   | 73x CPU faster |

## All Make Targets

```bash
# Setup
make lib               # Download ICICLE v3 libraries
make clean             # Clear Go build cache

# ICICLE v2 (stable - recommended)
make icicle-v2-small   # v2 MSM 2^7 = 128 elements
make icicle-v2-medium  # v2 MSM 2^15 = 32K elements
make icicle-v2-large   # v2 MSM 2^20 = 1M elements

# ICICLE v3 (unstable)
make icicle-small      # v3 MSM 2^7 = 128 elements
make icicle-medium     # v3 MSM 2^15 = 32K elements
make icicle-large      # v3 MSM 2^20 = 1M elements

# CPU vs GPU comparison
make compare           # CPU vs GPU (v3)
```

## Hardware

- GPU: NVIDIA GeForce RTX 4090
- CPU: AMD EPYC 7773X 64-Core Processor
- CUDA: 12.8
- Go: 1.24.9 linux/amd64
- GCC: 13.3.0
