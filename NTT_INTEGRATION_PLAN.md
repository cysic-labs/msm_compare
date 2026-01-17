# GPU NTT Integration Plan for PLONK Prover

## Executive Summary

This document outlines the plan to integrate ICICLE v2 GPU NTT into the gnark PLONK prover. Based on our MSM optimization report, NTT/FFT operations dominate **88% of prove time** while MSM only accounts for 4%. GPU NTT integration is the key to achieving significant speedup.

### Current State
| Component | Status | Notes |
|-----------|--------|-------|
| GPU MSM | **Working** | 5.5x faster than CPU (17ms vs 99ms at 2^20) |
| GPU KZG Open | **Working** | Uses GPU MSM for opening proofs |
| GPU NTT | **Disabled** | Infrastructure exists, needs real implementation |
| Overall Prove | 1.12x faster | Limited by CPU FFT bottleneck |

### Target Outcome
- Enable GPU NTT for the 88% of prove time currently on CPU
- Expected overall speedup: **2-5x** (depends on copy overhead)

---

## 1. Technical Analysis

### 1.1 FFT Usage in PLONK Prover

**Key FFT calls in `prove.go`:**

| Line | Operation | Domain | Description |
|------|-----------|--------|-------------|
| 989 | `p.ToCanonical(domain0)` | Small | Lagrange → Canonical (iFFT) |
| 1007 | `p.ToLagrange(domain0)` | Small | Canonical → Lagrange (FFT) |
| 1056 | `p.ToCanonical(domain0)` | Small | Final basis conversion |
| 1257 | `a.ToCanonical(domains[1])` | Large | Big domain conversion |
| Various | Coset FFT | Both | Polynomial division operations |

**Domain sizes:**
- `domain0`: Size = number of constraints (typically 2^20 for degree 20)
- `domain1`: Size = 4x or 8x of domain0 (2^22 or 2^23)

### 1.2 ICICLE v2 NTT API

```go
// Core NTT function
func Ntt[T any](
    scalars core.HostOrDeviceSlice,
    dir core.NTTDir,      // KForward or KInverse
    cfg *core.NTTConfig[T],
    results core.HostOrDeviceSlice,
) core.IcicleError

// Domain initialization (MUST call before NTT)
func InitDomain(
    primitiveRoot bn254.ScalarField,
    ctx cr.DeviceContext,
    fastTwiddles bool,
) core.IcicleError

// Configuration
type NTTConfig[T any] struct {
    Ctx                cr.DeviceContext
    CosetGen           T              // Coset generator (1 for normal, domain.FrMultiplicativeGen for coset)
    BatchSize          int32          // Number of NTTs to compute
    ColumnsBatch       bool           // Batch columns vs rows
    Ordering           Ordering       // KNN, KNR, KRN, KRR
    areInputsOnDevice  bool           // Auto-set based on input slice
    areOutputsOnDevice bool           // Auto-set based on output slice
    IsAsync            bool           // Async execution
    NttAlgorithm       NttAlgorithm   // Auto, Radix2, MixedRadix
}
```

### 1.3 Ordering Mapping (Critical!)

| gnark FFT | ICICLE Ordering | Description |
|-----------|-----------------|-------------|
| DIT + BitReverse | KRR | Bit-reversed in/out |
| DIT | KRN | Bit-reversed in, natural out |
| DIF + BitReverse | KNN | Natural in/out |
| DIF | KNR | Natural in, bit-reversed out |

**gnark PLONK typically uses DIF**, so mapping is:
- Normal FFT (DIF): `KNR`
- Coset FFT (DIF): `KNR` with `CosetGen = domain.FrMultiplicativeGen`

### 1.4 Data Format

**gnark fr.Element ↔ ICICLE ScalarField:**
- Both use Montgomery form internally
- Can use `core.HostSliceFromElements([]fr.Element)` directly
- No explicit Montgomery conversion needed for NTT (unlike MSM)

**Verified in test:**
```go
// Works directly with fr.Element
scalarsCopy := (core.HostSlice[fr.Element])(scalars[:testSize])
ntt.Ntt(scalarsCopy, core.KForward, &cfg, output)
```

---

## 2. Existing Infrastructure

### 2.1 Files to Modify

| File | Purpose | Changes Needed |
|------|---------|----------------|
| `prove_helper_gpu.go` | GPU FFT wrapper | Implement real GPU NTT |
| `fft_config.go` | FFT config | Enable `UseGPUFFT` |
| `provingkey.go` | GPU resources | Add NTT domain management |
| `fft_replacer.go` | FFT replacement | Update to use GPU path |

### 2.2 Current Implementation (Disabled)

**`prove_helper_gpu.go`** currently falls back to CPU:
```go
func (h *GPUProveHelper) FFT(coeffs *[]fr.Element, domain *fft.Domain, onCoset bool, dif bool) error {
    // Use CPU FFT (proven correct)
    if onCoset && dif {
        domain.FFT(*coeffs, fft.DIF, fft.OnCoset())
    }
    // ... CPU fallback
}
```

---

## 3. Implementation Plan

### Phase 1: Core GPU NTT Function (High Priority)

**Goal:** Implement working GPU NTT wrapper

**Tasks:**
1. Add domain initialization to `provingkey.go`:
   ```go
   type deviceInfo struct {
       // ... existing fields ...
       nttDomainInitialized bool
       nttMaxSize           uint64
   }

   func (pk *ProvingKey) InitNTTDomain(maxSize uint64) error {
       // Get primitive root from gnark
       rouMont, _ := fft.Generator(maxSize)
       rou := rouMont.Bits()
       rouIcicle := bn254.ScalarField{}
       limbs := core.ConvertUint64ArrToUint32Arr(rou[:])
       rouIcicle.FromLimbs(limbs)

       // Initialize ICICLE domain
       cfg := ntt.GetDefaultNttConfig()
       return ntt.InitDomain(rouIcicle, cfg.Ctx, false)
   }
   ```

2. Implement `NttGPU()` in `prove_helper_gpu.go`:
   ```go
   func (h *GPUProveHelper) NttGPU(
       coeffs []fr.Element,
       direction core.NTTDir,
       ordering core.Ordering,
       cosetGen *fr.Element,
   ) error {
       cfg := ntt.GetDefaultNttConfig()
       cfg.Ordering = ordering

       if cosetGen != nil {
           // Set coset generator
           cfg.CosetGen = convertToIcicleScalar(*cosetGen)
       }

       input := core.HostSliceFromElements(coeffs)
       output := make(core.HostSlice[fr.Element], len(coeffs))

       err := ntt.Ntt(input, direction, &cfg, output)
       if err.IcicleErrorCode != 0 {
           return fmt.Errorf("NTT failed: %v", err)
       }

       copy(coeffs, output)
       return nil
   }
   ```

3. Update `FFT()` and `FFTInverse()` to use GPU:
   ```go
   func (h *GPUProveHelper) FFT(coeffs *[]fr.Element, domain *fft.Domain, onCoset bool, dif bool) error {
       ordering := core.KNR  // DIF default
       if !dif {
           ordering = core.KRN  // DIT
       }

       var cosetGen *fr.Element
       if onCoset {
           cosetGen = &domain.FrMultiplicativeGen
       }

       return h.NttGPU(*coeffs, core.KForward, ordering, cosetGen)
   }
   ```

### Phase 2: Correctness Testing

**Goal:** Verify GPU NTT matches CPU results

**Tasks:**
1. Create test file `ntt_gpu_test.go`:
   - Test forward NTT (canonical → Lagrange)
   - Test inverse NTT (Lagrange → canonical)
   - Test coset NTT
   - Test with various sizes (2^10, 2^15, 2^20)
   - Compare results with CPU FFT bit-by-bit

2. Test ordering combinations:
   ```go
   testCases := []struct{
       gnarkDec fft.Decimation
       icicleOrd core.Ordering
   }{
       {fft.DIF, core.KNR},
       {fft.DIT, core.KRN},
   }
   ```

### Phase 3: Enable in Prover

**Goal:** Enable GPU NTT in actual prover

**Tasks:**
1. Update `fft_config.go`:
   ```go
   var DefaultFFTConfig = FFTConfig{
       UseGPUFFT: true,  // Enable GPU NTT
   }
   ```

2. Initialize NTT domain in `SetupDevicePointers()`:
   ```go
   func (pk *ProvingKey) SetupDevicePointers() error {
       // ... existing MSM setup ...

       // Initialize NTT domain for largest expected size
       maxNTTSize := uint64(pk.Domain0.Cardinality)
       if pk.Domain1 != nil && pk.Domain1.Cardinality > maxNTTSize {
           maxNTTSize = pk.Domain1.Cardinality
       }
       return pk.InitNTTDomain(maxNTTSize)
   }
   ```

3. Update `GPUProveHelper` initialization:
   ```go
   func NewGPUProveHelper(pk *ProvingKey) (*GPUProveHelper, error) {
       // ... existing code ...

       // Ensure NTT domain is initialized
       if !pk.deviceInfo.nttDomainInitialized {
           if err := pk.InitNTTDomain(maxDomainSize); err != nil {
               return nil, fmt.Errorf("init NTT domain: %w", err)
           }
       }

       return &GPUProveHelper{
           pk:        pk,
           fftConfig: FFTConfig{UseGPUFFT: true},
       }, nil
   }
   ```

### Phase 4: Performance Optimization

**Goal:** Optimize GPU NTT performance

**Tasks:**
1. **Batch NTT** - Process multiple polynomials in single GPU call:
   ```go
   func (h *GPUProveHelper) BatchNttGPU(coeffsList [][]fr.Element, ...) error {
       cfg.BatchSize = int32(len(coeffsList))
       // Flatten input, call NTT, unflatten output
   }
   ```

2. **Device-resident data** - Keep coefficients on GPU:
   ```go
   // Copy once, reuse for multiple NTTs
   coeffsDevice := copyToDevice(coeffs)
   ntt.Ntt(coeffsDevice, ..., resultDevice)  // Device → Device
   ```

3. **Async streams** - Overlap copy and compute:
   ```go
   cfg.IsAsync = true
   cfg.Ctx.Stream = &stream
   ```

### Phase 5: Benchmarking

**Goal:** Measure actual speedup

**Tasks:**
1. Create `benchmark_ntt.go`:
   - Benchmark GPU vs CPU NTT at various sizes
   - Measure H2D copy overhead
   - Measure NTT compute time
   - Report total speedup

2. Benchmark full prove:
   - Degree 20, 21, 22
   - Compare GPU (MSM+NTT) vs CPU
   - Report time breakdown

---

## 4. Risk Analysis

| Risk | Impact | Mitigation |
|------|--------|------------|
| Ordering mismatch | Wrong results | Extensive correctness testing |
| Domain initialization race | Crash | Use mutex (already exists) |
| Memory exhaustion | OOM | Pre-calculate memory needs |
| Copy overhead | Limited speedup | Device-resident data, batch ops |
| Coset generator issues | Wrong coset FFT | Test against gnark-crypto |

---

## 5. File Changes Summary

| File | Type | Changes |
|------|------|---------|
| `provingkey.go` | Modify | Add NTT domain init, cleanup |
| `prove_helper_gpu.go` | Modify | Implement real GPU NTT |
| `fft_config.go` | Modify | Enable UseGPUFFT |
| `fft_replacer.go` | Modify | Use GPU NTT path |
| `ntt_gpu.go` | New | Core NTT wrapper functions |
| `ntt_gpu_test.go` | New | Correctness tests |
| `benchmark_ntt.go` | New | Performance benchmarks |

---

## 6. Expected Timeline

| Phase | Description | Effort |
|-------|-------------|--------|
| Phase 1 | Core GPU NTT function | 1-2 days |
| Phase 2 | Correctness testing | 1 day |
| Phase 3 | Enable in prover | 0.5 day |
| Phase 4 | Performance optimization | 1-2 days |
| Phase 5 | Benchmarking | 0.5 day |
| **Total** | | **4-6 days** |

---

## 7. Success Criteria

1. **Correctness**: GPU NTT produces identical results to CPU FFT
2. **Performance**: GPU NTT faster than CPU for sizes >= 2^15
3. **Integration**: Full prover works with GPU NTT enabled
4. **Stability**: No crashes or memory leaks in extended testing
5. **Speedup**: Overall prove speedup >= 2x at degree 20

---

## 8. Quick Start Commands

```bash
# Test NTT correctness
cd /home/cysic/coding/plonky/lighter-prover-fork/external/gnark/backend/accelerated/icicle/plonk/bn254
go test -v -tags=icicle -run TestNTT

# Benchmark GPU vs CPU NTT
go test -v -tags=icicle -run BenchmarkNTT -bench=.

# Run full prover with GPU NTT
cd /home/cysic/coding/plonky/lighter-prover-fork/plonk_gpu_prove_test
make gpu20
```

---

## Appendix A: ICICLE v2 NTT Code Examples

### A.1 Basic NTT

```go
import (
    "github.com/ingonyama-zk/icicle/v2/wrappers/golang/core"
    bn254 "github.com/ingonyama-zk/icicle/v2/wrappers/golang/curves/bn254"
    ntt "github.com/ingonyama-zk/icicle/v2/wrappers/golang/curves/bn254/ntt"
)

// Initialize domain (once per device)
rouMont, _ := fft.Generator(1 << 20)  // For 2^20 size
rou := rouMont.Bits()
rouIcicle := bn254.ScalarField{}
limbs := core.ConvertUint64ArrToUint32Arr(rou[:])
rouIcicle.FromLimbs(limbs)

cfg := ntt.GetDefaultNttConfig()
ntt.InitDomain(rouIcicle, cfg.Ctx, false)

// Perform NTT
input := core.HostSliceFromElements(coeffs)  // []fr.Element
output := make(core.HostSlice[fr.Element], len(coeffs))
cfg.Ordering = core.KNR  // For DIF

ntt.Ntt(input, core.KForward, &cfg, output)
```

### A.2 Coset NTT

```go
// For coset FFT, set CosetGen to domain's multiplicative generator
cfg := ntt.GetDefaultNttConfig()

// Convert gnark coset generator to ICICLE format
cosetGenFr := domain.FrMultiplicativeGen
cosetGenBytes := cosetGenFr.Bytes()
var cosetGen [bn254.SCALAR_LIMBS]uint32
// ... convert bytes to limbs ...
cfg.CosetGen = cosetGen

ntt.Ntt(input, core.KForward, &cfg, output)
```

### A.3 Async NTT with Streams

```go
stream, _ := cr.CreateStream()

cfg := ntt.GetDefaultNttConfig()
cfg.IsAsync = true
cfg.Ctx.Stream = &stream

// Copy to device async
var deviceInput core.DeviceSlice
input.CopyToDeviceAsync(&deviceInput, stream, true)

// Allocate output on device
var deviceOutput core.DeviceSlice
deviceOutput.MallocAsync(size*elementSize, elementSize, stream)

// NTT on device
ntt.Ntt(deviceInput, core.KForward, &cfg, deviceOutput)

// Copy back async
output.CopyFromDeviceAsync(&deviceOutput, stream)

// Sync
cr.SynchronizeStream(&stream)
```
