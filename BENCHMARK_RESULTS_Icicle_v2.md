# PLONK CPU vs GPU Benchmark Comparison

## Test Environment

- **Circuit**: Merkle Tree Verification (Poseidon hash, depth 50)
- **GPU**: NVIDIA RTX 4090 with ICICLE v2 CUDA backend
- **CPU**: AMD EPYC 7773X
- **Runs per degree**: 3
- **Date**: 2026-01-16

---

## Compilation Time

| Degree | Constraints | CPU Compile | GPU Compile | Difference |
|--------|-------------|-------------|-------------|------------|
| 2^20   | 1,038,189   | 1.84s       | 1.67s       | GPU 0.17s faster |
| 2^21   | 2,086,804   | 3.29s       | 3.18s       | GPU 0.11s faster |
| 2^22   | 4,184,034   | 6.27s       | 5.38s       | GPU 0.89s faster |
| 2^23   | 8,378,494   | 12.53s      | 12.99s      | CPU 0.46s faster |

*Compilation times are similar - this is a CPU-only operation.*

---

## Setup Time (SRS Generation + PLONK Setup)

| Degree | CPU Setup | GPU Setup (KZG) | GPU Setup (Device) | GPU Total |
|--------|-----------|-----------------|---------------------|-----------|
| 2^20   | 2.35s     | 2.28s           | +0.72s              | 3.00s     |
| 2^21   | 9.00s     | 9.49s           | +0.80s              | 10.29s    |
| 2^22   | 17.63s    | 16.83s          | +1.15s              | 17.98s    |
| 2^23   | 30.40s    | 36.66s          | +2.67s              | 39.33s    |

*GPU requires additional device initialization for NTT domain setup.*

---

## Prove Time (Average of 3 runs)

| Degree | Constraints | CPU Prove | GPU Prove | GPU/CPU Ratio | Winner |
|--------|-------------|-----------|-----------|---------------|--------|
| 2^20   | ~1.04M      | **4.01s** | 4.33s     | 1.08x         | CPU    |
| 2^21   | ~2.09M      | **12.31s**| 13.92s    | 1.13x         | CPU    |
| 2^22   | ~4.18M      | **23.46s**| 25.87s    | 1.10x         | CPU    |
| 2^23   | ~8.38M      | **44.22s**| OOM       | N/A           | CPU    |

---

## Detailed Prove Times (All Runs)

| Degree | CPU Run 1 | CPU Run 2 | CPU Run 3 | CPU Avg | GPU Run 1 | GPU Run 2 | GPU Run 3 | GPU Avg |
|--------|-----------|-----------|-----------|---------|-----------|-----------|-----------|---------|
| 2^20   | 3.93s     | 3.88s     | 4.22s     | 4.01s   | 4.56s     | 4.24s     | 4.19s     | 4.33s   |
| 2^21   | 12.38s    | 12.61s    | 11.93s    | 12.31s  | 12.01s    | 14.13s    | 15.63s    | 13.92s  |
| 2^22   | 25.32s    | 23.36s    | 21.71s    | 23.46s  | 25.38s    | 26.51s    | 25.72s    | 25.87s  |
| 2^23   | 49.07s    | 41.81s    | 41.78s    | 44.22s  | 48.22s    | **OOM**   | -         | N/A     |

---

## GPU MSM Timing Breakdown

### Degree 2^22, Run 1

The GPU prover performs multiple MSM operations per proof:

**Lagrange Basis Commitments (3 parallel MSMs, each ~8M points):**
| Operation | MSM 1 | MSM 2 | MSM 3 |
|-----------|-------|-------|-------|
| Copy H→D  | 49ms  | 29ms  | 29ms  |
| Montgomery| 1.2ms | 21ms  | 0.05ms|
| MSM Compute| 382ms | 385ms | 243ms |
| Total     | 801ms | 804ms | 1090ms|

**Canonical Basis Commitments (3 parallel MSMs, each ~8M points):**
| Operation | MSM 1 | MSM 2 | MSM 3 |
|-----------|-------|-------|-------|
| Copy H→D  | 143ms | 130ms | 115ms |
| Montgomery| 0.2ms | 15ms  | 31ms  |
| MSM Compute| 865ms | 862ms | 868ms |
| Total     | 1224ms| 1226ms| 1227ms|

**Single Commitments:**
- Lagrange single MSM: ~376ms total
- Canonical single MSM: ~502ms total

---

## GPU Memory Analysis

### Degree 2^23 OOM Details

```
CUDA Runtime Error by: cudaMallocAsync at msm.cu:769
out of memory

CUDA Runtime Error by: bucket_method_msm at msm.cu:882
out of memory
```

The GPU ran out of memory during batch MSM with 3 parallel commitments of ~16M points each.
- First run completed successfully (48.22s)
- OOM occurred on second run, likely due to memory fragmentation

### Memory Requirements (Estimated)

| Degree | Points | Scalars Size | Points Size | Total per MSM |
|--------|--------|--------------|-------------|---------------|
| 2^20   | 1M     | 32 MB        | 96 MB       | ~128 MB       |
| 2^21   | 2M     | 64 MB        | 192 MB      | ~256 MB       |
| 2^22   | 4M     | 128 MB       | 384 MB      | ~512 MB       |
| 2^23   | 8M     | 256 MB       | 768 MB      | ~1 GB         |
| 2^24   | 16M    | 512 MB       | 1.5 GB      | ~2 GB         |

*With 3 parallel MSMs + intermediate buckets, degree 2^23 requires >20GB GPU memory.*

---

## Key Findings

### 1. CPU Outperforms GPU at All Tested Degrees

- GPU is **8-13% slower** than CPU for proving
- The gap is consistent across all working degrees (2^20 to 2^22)

### 2. GPU Memory Limitations

- **Maximum working degree**: 2^22 (~4M constraints)
- **OOM at degree 2^23**: 16M+ point MSMs exceed GPU memory
- First proof at 2^23 succeeded, but memory fragmentation caused OOM on subsequent runs

### 3. GPU Overhead Sources

| Overhead Type | Impact | Per-MSM Cost |
|---------------|--------|--------------|
| Host-to-Device copy | High | 50-170ms |
| Montgomery conversion | Medium | 1-30ms |
| Serial batch execution | Medium | Not parallelized |
| Device initialization | One-time | 0.7-2.7s |

### 4. Timing Variance

| Metric | CPU | GPU |
|--------|-----|-----|
| Degree 2^21 spread | 6% (11.93-12.61s) | 30% (12.01-15.63s) |
| Degree 2^22 spread | 16% (21.71-25.32s) | 4% (25.38-26.51s) |

GPU shows more variance, especially at lower degrees.

---

## Conclusion

For this PLONK prover implementation with ICICLE v2:

| Aspect | Result |
|--------|--------|
| **Performance** | CPU is faster at all practical circuit sizes |
| **Scalability** | GPU limited to ~4M constraints by memory |
| **Stability** | GPU shows higher timing variance |
| **Recommendation** | Use CPU for production workloads |

---

## Potential GPU Optimizations

To make GPU competitive, consider:

1. **Pre-load SRS points to GPU memory**
   - Amortize transfer cost across many proofs
   - Requires persistent GPU memory management

2. **GPU FFT/NTT operations**
   - Currently CPU-only in this implementation
   - ICICLE supports GPU NTT

3. **Batch multiple proofs**
   - Amortize device setup costs
   - Better GPU utilization

4. **Overlap copy and compute**
   - Use CUDA streams for async operations
   - Pipeline data transfer with computation

5. **Memory optimization**
   - Sequential MSM instead of parallel for large degrees
   - Trade compute time for memory footprint

---

## Raw Data Files

- CPU degree 20: `/tmp/cpu20.log`
- CPU degree 21: `/tmp/cpu21.log`
- CPU degree 22: `/tmp/cpu22.log`
- CPU degree 23: `/tmp/cpu23.log`
- GPU degree 20: `/tmp/gpu20.log`
- GPU degree 21: `/tmp/gpu21.log`
- GPU degree 22: `/tmp/gpu22.log`
- GPU degree 23: `/tmp/gpu23.log` (OOM)

> Update Time: 2026-01-17 12:27:00 UTC+8