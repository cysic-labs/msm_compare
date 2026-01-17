# GPU NTT Investigation Findings

## Summary

**ICICLE v2 GPU NTT is faster than CPU FFT.** The initial tests with icicle-gnark v3 showed GPU NTT being slower than CPU, but this was due to a bug where v3 falls back to CPU execution even when CUDA device is selected.

## Performance Results

| Size | CPU FFT (gnark-crypto) | GPU NTT (ICICLE v2) | Speedup |
|------|------------------------|---------------------|---------|
| 2^18 (262K) | 5.9ms | 2.0ms | **3x faster** |
| 2^20 (1M) | 14.7ms | 3.5ms | **4.2x faster** |
| 2^22 (4M) | 56ms | 44ms | **1.3x faster** |

## Problem with icicle-gnark v3

The icicle-gnark v3 library has a backend system that loads CUDA support dynamically. However, the NTT implementation in this backend doesn't properly dispatch to GPU - it runs on CPU even when CUDA device is selected.

Evidence:
- NTT with v3 took ~60ms for 2^18 elements (same as CPU)
- Async call blocked for full duration, sync was instant (CPU pattern)
- GPU utilization stayed at 0% during NTT operations
- MSM had the same issue - took 5.8 seconds instead of 20ms for 2^20 elements

## Solution: Use ICICLE v2

ICICLE v2 with static CUDA libraries works correctly. The static libraries have proper C-style symbols (`bn254_ntt_cuda`, `bn254_initialize_domain`, `bn254_release_domain`) that the Go wrapper expects.

### Required Libraries

Static libraries location:
```
/home/cysic/coding/plonky/icicle/examples/golang/msm/lib/libingo_field_bn254.a
```

### CGO Flags

```bash
CGO_CFLAGS="-I/usr/local/cuda/include -I/home/cysic/coding/plonky/icicle/icicle/include"
CGO_LDFLAGS="-L/home/cysic/coding/plonky/icicle/examples/golang/msm/lib -licicle_field_bn254 -lstdc++ -lcudart -L/usr/local/cuda/lib64"
```

## How to Run the Benchmark

```bash
cd /home/cysic/coding/plonky/msm_compare

CGO_CFLAGS="-I/usr/local/cuda/include -I/home/cysic/coding/plonky/icicle/icicle/include" \
CGO_LDFLAGS="-L/home/cysic/coding/plonky/icicle/examples/golang/msm/lib -licicle_field_bn254 -lstdc++ -lcudart -L/usr/local/cuda/lib64" \
go run -tags=icicle ntt_v2_benchmark.go -size 20 -runs 5
```

Options:
- `-size N` : NTT size as power of 2 (default: 18)
- `-runs N` : Number of benchmark runs (default: 3)

## Benchmark Code

See `ntt_v2_benchmark.go` in this directory.

## Key Imports for ICICLE v2 NTT

```go
import (
    "github.com/ingonyama-zk/icicle/v2/wrappers/golang/core"
    cr "github.com/ingonyama-zk/icicle/v2/wrappers/golang/cuda_runtime"
    "github.com/ingonyama-zk/icicle/v2/wrappers/golang/curves/bn254"
    bn254Ntt "github.com/ingonyama-zk/icicle/v2/wrappers/golang/curves/bn254/ntt"
)
```

## Conclusion

For GPU-accelerated NTT in Go, use ICICLE v2 with static CUDA libraries instead of icicle-gnark v3. The v2 implementation provides 3-4x speedup over CPU FFT for typical sizes (2^18 to 2^20).
