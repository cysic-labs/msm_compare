//go:build icicle

package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr/fft"

	icicle_bn254 "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254"
	icicle_core "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/core"
	icicle_ntt "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254/ntt"
	icicle_runtime "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/runtime"
)

func main() {
	sizeLog := flag.Int("size", 18, "NTT size as power of 2")
	runs := flag.Int("runs", 3, "Number of runs")
	flag.Parse()

	size := 1 << *sizeLog

	fmt.Printf("NTT size: 2^%d = %d elements\n", *sizeLog, size)
	fmt.Printf("Runs: %d\n", *runs)
	fmt.Println("========================================")
	fmt.Println()

	// Load backend
	fmt.Println("Loading ICICLE backend...")
	err := icicle_runtime.LoadBackendFromEnvOrDefault()
	if err != icicle_runtime.Success {
		panic(fmt.Sprintf("Failed to load backend: %v", err))
	}

	// Set device to CUDA
	deviceCuda := icicle_runtime.CreateDevice("CUDA", 0)
	if icicle_runtime.IsDeviceAvailable(&deviceCuda) {
		icicle_runtime.SetDevice(&deviceCuda)
		fmt.Println("✓ Using CUDA device (GPU-0)")
	} else {
		fmt.Println("⚠ CUDA not available, using CPU backend")
	}

	// Print active device
	activeDevice, _ := icicle_runtime.GetActiveDevice()
	fmt.Printf("Active device: %s\n", activeDevice.GetDeviceType())
	fmt.Println()

	// Generate scalars
	fmt.Printf("Generating %d random scalars...\n", size)
	startGen := time.Now()
	scalars := icicle_bn254.GenerateScalars(size)
	genTime := time.Since(startGen)
	fmt.Printf("✓ Generated in %v\n", genTime)
	fmt.Println()

	// Initialize NTT domain
	fmt.Println("Initializing NTT domain...")
	domain := fft.NewDomain(uint64(size))
	rouIcicle := icicle_bn254.ScalarField{}
	rouBits := domain.Generator.Bits()
	limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
	rouIcicle.FromLimbs(limbs)

	initCfg := icicle_core.GetDefaultNTTInitDomainConfig()
	initErr := icicle_ntt.InitDomain(rouIcicle, initCfg)
	if initErr != icicle_runtime.Success {
		panic(fmt.Sprintf("InitDomain failed: %v", initErr))
	}
	fmt.Println("✓ NTT domain initialized")
	fmt.Println()

	// Copy to device
	fmt.Println("Copying data to device...")
	var inputDevice, outputDevice icicle_core.DeviceSlice
	scalars.CopyToDevice(&inputDevice, true)
	scalars.CopyToDevice(&outputDevice, true)
	fmt.Println("✓ Data copied to device")
	fmt.Println()

	// Warmup
	fmt.Println("Running warm-up...")
	nttCfg := icicle_ntt.GetDefaultNttConfig()
	for w := 0; w < 3; w++ {
		icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfg, outputDevice)
	}
	fmt.Println("✓ Warm-up complete")
	fmt.Println()

	// Benchmark runs
	fmt.Printf("Running %d benchmark iterations...\n", *runs)
	var totalTime time.Duration

	for i := 0; i < *runs; i++ {
		fmt.Printf("  Run %d/%d: ", i+1, *runs)

		// Reset input
		scalars.CopyToDevice(&inputDevice, true)

		// Execute NTT
		start := time.Now()
		nttErr := icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfg, outputDevice)
		duration := time.Since(start)

		if nttErr != icicle_runtime.Success {
			panic(fmt.Sprintf("NTT failed: %v", nttErr))
		}

		totalTime += duration
		fmt.Printf("%v\n", duration)
	}

	avgTime := totalTime / time.Duration(*runs)

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Results")
	fmt.Println("========================================")
	fmt.Printf("Average NTT time: %v\n", avgTime)
	fmt.Printf("Throughput: %.2f NTTs/second\n", float64(*runs)/totalTime.Seconds())
	fmt.Println("========================================")

	// Cleanup
	inputDevice.Free()
	outputDevice.Free()
	icicle_ntt.ReleaseDomain()

	// Test async behavior
	fmt.Println()
	fmt.Println("Testing async behavior...")

	// Reinitialize
	icicle_ntt.InitDomain(rouIcicle, initCfg)
	scalars.CopyToDevice(&inputDevice, true)
	scalars.CopyToDevice(&outputDevice, true)

	// Create stream
	stream, _ := icicle_runtime.CreateStream()
	defer icicle_runtime.DestroyStream(stream)

	asyncCfg := icicle_ntt.GetDefaultNttConfig()
	asyncCfg.StreamHandle = stream
	asyncCfg.IsAsync = true

	// Warmup
	for w := 0; w < 3; w++ {
		icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &asyncCfg, outputDevice)
	}
	icicle_runtime.SynchronizeStream(stream)

	// Test async
	scalars.CopyToDevice(&inputDevice, true)
	asyncStart := time.Now()
	icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &asyncCfg, outputDevice)
	asyncCallTime := time.Since(asyncStart)

	syncStart := time.Now()
	icicle_runtime.SynchronizeStream(stream)
	syncTime := time.Since(syncStart)

	fmt.Printf("  Async call time: %v\n", asyncCallTime)
	fmt.Printf("  Stream sync time: %v\n", syncTime)
	fmt.Printf("  Total: %v\n", asyncCallTime+syncTime)

	if asyncCallTime < 5*time.Millisecond {
		fmt.Println("  ✓ GPU NTT: async call returns quickly")
	} else {
		fmt.Println("  ✗ CPU NTT: async call blocks (likely CPU fallback)")
	}

	inputDevice.Free()
	outputDevice.Free()
	icicle_ntt.ReleaseDomain()
}
