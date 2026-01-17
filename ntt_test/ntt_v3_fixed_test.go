//go:build icicle

package ntt_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/fft"

	icicle_bn254 "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254"
	icicle_core "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/core"
	icicle_ntt "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254/ntt"
	icicle_runtime "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/runtime"
)

func init() {
	os.Setenv("ICICLE_BACKEND_INSTALL_DIR", "/home/cysic/coding/plonky/msm_compare/lib/backend")
}

// TestNTTV3Fixed tests NTT following the exact same pattern as working MSM example
func TestNTTV3Fixed(t *testing.T) {
	// Load backend - same as MSM
	fmt.Println("Loading ICICLE backend...")
	err := icicle_runtime.LoadBackendFromEnvOrDefault()
	if err != icicle_runtime.Success {
		t.Fatalf("Failed to load backend: %v", err)
	}

	// Check and set device - same as MSM
	deviceCuda := icicle_runtime.CreateDevice("CUDA", 0)
	if icicle_runtime.IsDeviceAvailable(&deviceCuda) {
		icicle_runtime.SetDevice(&deviceCuda)
		fmt.Println("✓ Using CUDA device (GPU-0)")
	} else {
		t.Fatal("CUDA not available")
	}

	sizes := []int{18, 20, 22}
	runs := 5

	fmt.Println("\n=== NTT v3 Fixed Pattern Benchmark ===")

	for _, logSize := range sizes {
		size := 1 << logSize
		fmt.Printf("\nSize 2^%d (%d elements):\n", logSize, size)

		// Generate random scalars using ICICLE's native function - like MSM does
		fmt.Println("  Generating scalars...")
		scalars := icicle_bn254.GenerateScalars(size)

		// Also generate gnark-crypto data for CPU comparison
		frData := make([]fr.Element, size)
		for i := range frData {
			frData[i].SetRandom()
		}

		// CPU FFT benchmark
		domain := fft.NewDomain(uint64(size))
		cpuData := make([]fr.Element, size)
		var cpuTotal time.Duration
		for r := 0; r < runs; r++ {
			copy(cpuData, frData)
			start := time.Now()
			domain.FFT(cpuData, fft.DIF)
			cpuTotal += time.Since(start)
		}
		cpuAvg := cpuTotal / time.Duration(runs)
		fmt.Printf("  CPU FFT: %v\n", cpuAvg)

		// Initialize NTT domain
		rouIcicle := icicle_bn254.ScalarField{}
		rouBits := domain.Generator.Bits()
		limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
		rouIcicle.FromLimbs(limbs)

		initCfg := icicle_core.GetDefaultNTTInitDomainConfig()
		initErr := icicle_ntt.InitDomain(rouIcicle, initCfg)
		if initErr != icicle_runtime.Success {
			t.Fatalf("InitDomain failed: %v", initErr)
		}
		fmt.Println("  NTT domain initialized")

		// Copy scalars to device - same pattern as MSM
		var scalarsDevice icicle_core.DeviceSlice
		scalars.CopyToDevice(&scalarsDevice, true)

		// Allocate output on device
		var outputDevice icicle_core.DeviceSlice
		scalars.CopyToDevice(&outputDevice, true)

		fmt.Println("  Data copied to device")

		// Configure NTT - use default coset generator (should be One)
		nttCfg := icicle_ntt.GetDefaultNttConfig()

		// Warmup - same as MSM
		fmt.Println("  Running warmup...")
		for w := 0; w < 3; w++ {
			icicle_ntt.Ntt(scalarsDevice, icicle_core.KForward, &nttCfg, outputDevice)
		}

		// Benchmark GPU NTT
		fmt.Println("  Running benchmark...")
		var gpuTotal time.Duration
		for r := 0; r < runs; r++ {
			// Reset input
			scalars.CopyToDevice(&scalarsDevice, true)

			start := time.Now()
			nttErr := icicle_ntt.Ntt(scalarsDevice, icicle_core.KForward, &nttCfg, outputDevice)
			gpuTotal += time.Since(start)

			if nttErr != icicle_runtime.Success {
				t.Fatalf("NTT failed: %v", nttErr)
			}
		}
		gpuAvg := gpuTotal / time.Duration(runs)
		fmt.Printf("  GPU NTT: %v\n", gpuAvg)

		speedup := float64(cpuAvg) / float64(gpuAvg)
		fmt.Printf("  Speedup: %.2fx\n", speedup)
		if speedup > 1 {
			fmt.Println("  Winner: GPU")
		} else {
			fmt.Println("  Winner: CPU")
		}

		// Cleanup
		scalarsDevice.Free()
		outputDevice.Free()
		icicle_ntt.ReleaseDomain()
	}
}

// TestNTTV3WithStream tests NTT using explicit stream for proper async
func TestNTTV3WithStream(t *testing.T) {
	// Load backend
	err := icicle_runtime.LoadBackendFromEnvOrDefault()
	if err != icicle_runtime.Success {
		t.Fatalf("Failed to load backend: %v", err)
	}

	deviceCuda := icicle_runtime.CreateDevice("CUDA", 0)
	icicle_runtime.SetDevice(&deviceCuda)

	size := 1 << 20 // 2^20
	fmt.Printf("=== Stream-based NTT Test (2^20 = %d elements) ===\n\n", size)

	// Generate scalars
	scalars := icicle_bn254.GenerateScalars(size)

	// Initialize domain
	domain := fft.NewDomain(uint64(size))
	rouIcicle := icicle_bn254.ScalarField{}
	rouBits := domain.Generator.Bits()
	limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
	rouIcicle.FromLimbs(limbs)

	initCfg := icicle_core.GetDefaultNTTInitDomainConfig()
	icicle_ntt.InitDomain(rouIcicle, initCfg)
	defer icicle_ntt.ReleaseDomain()

	// Copy to device
	var scalarsDevice, outputDevice icicle_core.DeviceSlice
	scalars.CopyToDevice(&scalarsDevice, true)
	scalars.CopyToDevice(&outputDevice, true)
	defer scalarsDevice.Free()
	defer outputDevice.Free()

	// Create stream
	stream, streamErr := icicle_runtime.CreateStream()
	if streamErr != icicle_runtime.Success {
		t.Fatalf("Failed to create stream: %v", streamErr)
	}
	defer icicle_runtime.DestroyStream(stream)
	fmt.Println("Created CUDA stream")

	// Configure NTT with stream
	nttCfg := icicle_ntt.GetDefaultNttConfig()
	nttCfg.StreamHandle = stream
	nttCfg.IsAsync = true

	// Warmup
	for w := 0; w < 3; w++ {
		icicle_ntt.Ntt(scalarsDevice, icicle_core.KForward, &nttCfg, outputDevice)
	}
	icicle_runtime.SynchronizeStream(stream)

	// Test async behavior
	fmt.Println("\nAsync NTT with stream:")
	scalars.CopyToDevice(&scalarsDevice, true)

	asyncStart := time.Now()
	icicle_ntt.Ntt(scalarsDevice, icicle_core.KForward, &nttCfg, outputDevice)
	asyncCallTime := time.Since(asyncStart)
	fmt.Printf("  NTT call time: %v\n", asyncCallTime)

	syncStart := time.Now()
	icicle_runtime.SynchronizeStream(stream)
	syncTime := time.Since(syncStart)
	fmt.Printf("  Stream sync time: %v\n", syncTime)
	fmt.Printf("  Total: %v\n", asyncCallTime+syncTime)

	// Analysis
	fmt.Println("\nAnalysis:")
	if asyncCallTime < 10*time.Millisecond && syncTime > 10*time.Millisecond {
		fmt.Println("  ✓ GPU NTT detected: async call returns quickly, sync waits for GPU")
	} else if asyncCallTime > 50*time.Millisecond && syncTime < 10*time.Millisecond {
		fmt.Println("  ✗ CPU NTT detected: async call blocks (runs on CPU), sync is instant")
	} else {
		fmt.Printf("  Unclear (call: %v, sync: %v)\n", asyncCallTime, syncTime)
	}
}
