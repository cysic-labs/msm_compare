# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is a benchmarking tool that compares CPU vs GPU performance for Multi-Scalar Multiplication (MSM) operations on the BN254 elliptic curve. It supports both ICICLE v2 and v3 GPU acceleration libraries with CUDA backend, plus gnark-crypto for CPU operations.

**Key Finding**: ICICLE v3 has severe timing instability (20ms to 6-9 seconds). **ICICLE v2 is recommended** for stable, predictable performance (~18ms typical).

| Version | Typical MSM (2^20) | Worst Case | Variation |
|---------|-------------------|------------|-----------|
| ICICLE v2 | 17-20ms | ~160ms | ~8x |
| ICICLE v3 | 20ms | 6-9 seconds | 300-400x |

## Build and Run Commands

### Initial Setup
```bash
make lib  # Download ICICLE BN254 libraries (required first time)
```

This downloads 6 shared libraries to `lib/` and `lib/backend/`:
- `libicicle_curve_bn254.so`, `libicicle_field_bn254.so`, `libicicle_device.so`
- Backend CUDA libraries in `lib/backend/`

### Running Tests

All Go commands require the `-tags=icicle` build tag.

**ICICLE v2 (Recommended - Stable)**:
```bash
make icicle-v2-small   # 2^7 = 128 elements
make icicle-v2-medium  # 2^15 = 32K elements
make icicle-v2-large   # 2^20 = 1M elements (recommended benchmark)
```

**ICICLE v3 (Unstable - has timing issues)**:
```bash
make icicle-small    # 2^7 = 128 elements
make icicle-medium   # 2^15 = 32K elements
make icicle-large    # 2^20 = 1M elements
```

**CPU vs GPU Comparison** (range of sizes):
```bash
make compare  # Tests 2^18 to 2^20, 3 runs each (uses v3)
```

### Cleanup
```bash
make clean  # Clear Go build cache
```

## Architecture

### Environment Configuration

The Makefile sets critical environment variables for ICICLE:
- `ICICLE_BACKEND_INSTALL_DIR=lib/backend/` - Location of CUDA backend libraries
- `LD_LIBRARY_PATH` - Includes `lib/`, `lib/backend/`, and CUDA paths
- `CGO_LDFLAGS` - Links all required ICICLE libraries with rpath for runtime

### Main Programs

**1. `icicle_official_example_v2.go`** - ICICLE v2 benchmark (Recommended)
- Uses ICICLE v2 with direct CUDA runtime
- Stable, predictable performance (~18ms for 2^20)
- Measures copy time and MSM compute time separately

**2. `icicle_official_example.go`** - ICICLE v3 benchmark
- Based on official ICICLE v3 documentation
- Has severe timing instability (20ms to 6-9 seconds)
- Uses ICICLE v3 backend abstraction layer

**3. `main.go`** - Full CPU vs GPU comparison
- Compares gnark-crypto CPU MSM against ICICLE v3 GPU MSM
- Tests multiple sizes in a single run
- Optional detailed timing breakdown with `-detailed` flag

### MSM Execution Flow

**CPU Path** (`runCPUMSM`):
- Direct call to `bn254.G1Affine.MultiExp()` from gnark-crypto
- Single-threaded or multi-threaded depending on gnark-crypto config

**GPU Path** (`runGPUMSM`):
1. Copy bases (G1 points) from host to device
2. Copy scalars from host to device
3. Convert bases from Montgomery form (GPU operation)
4. Convert scalars from Montgomery form (GPU operation)
5. Execute MSM computation on GPU
6. Result automatically copied back to host (implicit in ICICLE v3)
7. Convert ICICLE projective result to gnark affine format

**Timing Breakdown** (when `-detailed` flag used):
- Bases H2D, Scalars H2D
- Bases Montgomery conversion, Scalars Montgomery conversion
- MSM compute time
- Result D2H (device to host)

### Key Types

- `MSMTest` - Test configuration (degree, size, description)
- `GPUTimingBreakdown` - Detailed GPU operation timings
- Uses `bn254.G1Affine` (gnark-crypto) for CPU operations
- Uses `icicle_bn254.Projective` for GPU results, converted to gnark format

### Dependencies

- `github.com/consensys/gnark-crypto` v0.12.1 - CPU MSM and BN254 types
- `github.com/ingonyama-zk/icicle/v2` - GPU acceleration (stable, recommended)
- `github.com/ingonyama-zk/icicle-gnark/v3` v3.2.2 - GPU acceleration (unstable)

## Hardware Requirements

- **GPU**: NVIDIA GPU with CUDA support (tested on RTX 4090)
- **CUDA**: Version 12.8 (check with `nvcc --version`)
- **CPU**: Multi-core recommended (tested on AMD EPYC 7773X)
- **Go**: 1.24.9+ (uses Go 1.24.9 in go.mod)

## ICICLE v3 Performance Instability

**Note: This issue affects ICICLE v3 only. ICICLE v2 is stable.**

The v3 GPU performance varies by 30-400x due to power state switching:
- **P0 state (2520 MHz)**: Fast runs (17-180ms for 2^20 elements)
- **P5 state (1400-1700 MHz)**: Slow runs (5-7 seconds)

Root cause: ICICLE v3 MSM kernel only uses ~17% of GPU SM utilization, causing random power state transitions.

**Recommended solution**: Use ICICLE v2 instead (`make icicle-v2-large`).

**Alternative - Lock GPU clocks (requires sudo):**
```bash
sudo nvidia-smi -lgc 2520,3105 -i 0  # Lock GPU clocks
sudo nvidia-smi -lmc 10501 -i 0      # Lock memory clocks
```

See `GPU_TIMING_ANALYSIS.md` and `WHY_LOW_GPU_UTILIZATION.md` for detailed investigation.

## Important Notes

- All Go commands MUST include `-tags=icicle` build tag
- Libraries must be downloaded with `make lib` before first run
- GPU performance is highly variable - run multiple iterations (or lock clocks)
- Montgomery conversions are significant overhead for GPU path
- In production (e.g., PLONK proving), bases are pre-loaded to GPU, reducing per-MSM overhead
