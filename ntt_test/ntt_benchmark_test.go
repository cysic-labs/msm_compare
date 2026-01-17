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
	// Set environment for ICICLE backend
	os.Setenv("ICICLE_BACKEND_INSTALL_DIR", "/home/cysic/coding/plonky/msm_compare/lib/backend")
}

// TestDeviceInfo checks ICICLE device configuration
func TestDeviceInfo(t *testing.T) {
	loadErr := icicle_runtime.LoadBackendFromEnvOrDefault()
	if loadErr != icicle_runtime.Success {
		t.Fatalf("Failed to load backend: %v", loadErr)
	}

	// List registered devices
	devices, _ := icicle_runtime.GetRegisteredDevices()
	fmt.Printf("Registered devices: %v\n", devices)

	// Try to get CUDA device info
	device := icicle_runtime.CreateDevice("CUDA", 0)
	fmt.Printf("Device type: %s\n", device.GetDeviceType())

	// Check if device is active
	done := make(chan bool, 1)
	icicle_runtime.RunOnDevice(&device, func(args ...any) {
		defer func() { done <- true }()
		activeDevice, _ := icicle_runtime.GetActiveDevice()
		fmt.Printf("Active device type: %s\n", activeDevice.GetDeviceType())

		// Check if CUDA device is available
		cudaDev := icicle_runtime.CreateDevice("CUDA", 0)
		isAvailable := icicle_runtime.IsDeviceAvailable(&cudaDev)
		fmt.Printf("CUDA device available: %v\n", isAvailable)
	})
	<-done
}

// TestNTTSingleRun tests a single NTT execution with detailed timing
func TestNTTSingleRun(t *testing.T) {
	loadErr := icicle_runtime.LoadBackendFromEnvOrDefault()
	if loadErr != icicle_runtime.Success {
		t.Fatalf("Failed to load backend: %v", loadErr)
	}

	device := icicle_runtime.CreateDevice("CUDA", 0)

	sizes := []int{18, 20, 22}

	fmt.Println("=== Single NTT Execution Timing ===")
	fmt.Println()

	for _, logSize := range sizes {
		size := 1 << logSize
		fmt.Printf("Size 2^%d (%d elements):\n", logSize, size)

		data := make([]fr.Element, size)
		for i := range data {
			data[i].SetRandom()
		}

		domain := fft.NewDomain(uint64(size))

		// CPU FFT single run
		cpuData := make([]fr.Element, size)
		copy(cpuData, data)
		cpuStart := time.Now()
		domain.FFT(cpuData, fft.DIF)
		cpuTime := time.Since(cpuStart)
		fmt.Printf("  CPU FFT: %v\n", cpuTime)

		done := make(chan bool, 1)
		icicle_runtime.RunOnDevice(&device, func(args ...any) {
			defer func() { done <- true }()

			// Check device before domain init
			activeDevBefore, _ := icicle_runtime.GetActiveDevice()
			fmt.Printf("  Active device before init: %s\n", activeDevBefore.GetDeviceType())

			// Initialize domain
			rouIcicle := icicle_bn254.ScalarField{}
			rouBits := domain.Generator.Bits()
			limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
			rouIcicle.FromLimbs(limbs)
			cfg := icicle_core.GetDefaultNTTInitDomainConfig()
			initRes := icicle_ntt.InitDomain(rouIcicle, cfg)
			fmt.Printf("  InitDomain result: %v\n", initRes)
			defer icicle_ntt.ReleaseDomain()

			// Check device after domain init
			activeDevAfter, _ := icicle_runtime.GetActiveDevice()
			fmt.Printf("  Active device after init: %s\n", activeDevAfter.GetDeviceType())

			// Copy data to GPU
			hostSlice := icicle_core.HostSliceFromElements(data)
			var inputDevice, outputDevice icicle_core.DeviceSlice
			copyStart := time.Now()
			hostSlice.CopyToDevice(&inputDevice, true)
			hostSlice.CopyToDevice(&outputDevice, true)
			copyTime := time.Since(copyStart)
			fmt.Printf("  H2D copy: %v\n", copyTime)

			// Warmup with explicit CUDA config
			var cosetGen icicle_bn254.ScalarField
			nttCfg := icicle_core.GetDefaultNTTConfig(cosetGen)
			nttCfg.Ordering = icicle_core.KNR

			// Try setting CUDA-specific options via extension config
			// This might enable GPU NTT vs CPU fallback
			fmt.Printf("  Config areInputsOnDevice will be set by NttCheck\n")

			for w := 0; w < 3; w++ {
				icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfg, outputDevice)
			}
			hostSlice.CopyFromDevice(&outputDevice)
			fmt.Printf("  Warmup done\n")

			// Time single NTT (sync mode)
			hostSlice.CopyToDevice(&inputDevice, true) // Reset
			nttStart := time.Now()
			icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfg, outputDevice)
			nttTime := time.Since(nttStart)
			fmt.Printf("  GPU NTT sync (before D2H): %v\n", nttTime)

			// Sync
			syncStart := time.Now()
			hostSlice.CopyFromDevice(&outputDevice)
			syncTime := time.Since(syncStart)
			fmt.Printf("  D2H copy: %v\n", syncTime)
			fmt.Printf("  GPU total (sync mode): %v\n", nttTime+syncTime)

			// Try async mode
			nttCfgAsync := icicle_core.GetDefaultNTTConfig(cosetGen)
			nttCfgAsync.Ordering = icicle_core.KNR
			nttCfgAsync.IsAsync = true
			hostSlice.CopyToDevice(&inputDevice, true) // Reset

			asyncStart := time.Now()
			icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfgAsync, outputDevice)
			asyncCallTime := time.Since(asyncStart)
			fmt.Printf("  GPU NTT async (call return): %v\n", asyncCallTime)

			// Now sync
			asyncSyncStart := time.Now()
			icicle_runtime.DeviceSynchronize()
			asyncSyncTime := time.Since(asyncSyncStart)
			fmt.Printf("  GPU sync: %v\n", asyncSyncTime)
			fmt.Printf("  GPU total (async mode): %v\n", asyncCallTime+asyncSyncTime)

			inputDevice.Free()
			outputDevice.Free()
		})
		<-done
		fmt.Println()
	}
}

// TestNTTBenchmark compares CPU FFT vs GPU NTT at various sizes
func TestNTTBenchmark(t *testing.T) {
	// Load ICICLE backend
	loadErr := icicle_runtime.LoadBackendFromEnvOrDefault()
	if loadErr != icicle_runtime.Success {
		t.Fatalf("Failed to load ICICLE backend: %v", loadErr)
	}

	device := icicle_runtime.CreateDevice("CUDA", 0)

	sizes := []int{18, 19, 20, 21, 22, 23}
	runs := 3

	fmt.Println("=== CPU FFT vs GPU NTT Benchmark (v3) ===")
	fmt.Println()

	for _, logSize := range sizes {
		size := 1 << logSize
		t.Run(fmt.Sprintf("Size_2^%d", logSize), func(t *testing.T) {
			// Generate random data
			data := make([]fr.Element, size)
			for i := range data {
				data[i].SetRandom()
			}

			// Create FFT domain for CPU
			domain := fft.NewDomain(uint64(size))

			// Benchmark CPU FFT
			cpuData := make([]fr.Element, size)
			var cpuTotal time.Duration
			for r := 0; r < runs; r++ {
				copy(cpuData, data)
				start := time.Now()
				domain.FFT(cpuData, fft.DIF)
				cpuTotal += time.Since(start)
			}
			cpuAvg := cpuTotal / time.Duration(runs)

			// Benchmark GPU NTT
			var gpuTotal time.Duration
			var gpuErr error

			done := make(chan bool, 1)
			icicle_runtime.RunOnDevice(&device, func(args ...any) {
				defer func() { done <- true }()

				// Initialize NTT domain for GPU
				rouIcicle := icicle_bn254.ScalarField{}
				rouBits := domain.Generator.Bits()
				limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
				rouIcicle.FromLimbs(limbs)

				cfg := icicle_core.GetDefaultNTTInitDomainConfig()
				initResult := icicle_ntt.InitDomain(rouIcicle, cfg)
				if initResult != icicle_runtime.Success {
					gpuErr = fmt.Errorf("Failed to init NTT domain: %v", initResult)
					return
				}
				defer icicle_ntt.ReleaseDomain()

				for r := 0; r < runs; r++ {
					gpuData := make([]fr.Element, size)
					copy(gpuData, data)

					start := time.Now()

					// Copy to device
					hostSlice := icicle_core.HostSliceFromElements(gpuData)
					var deviceSlice icicle_core.DeviceSlice
					hostSlice.CopyToDevice(&deviceSlice, true)

					// Run NTT
					var cosetGen icicle_bn254.ScalarField
					nttCfg := icicle_core.GetDefaultNTTConfig(cosetGen)
					nttCfg.Ordering = icicle_core.KNR

					err := icicle_ntt.Ntt(deviceSlice, icicle_core.KForward, &nttCfg, deviceSlice)
					if err != icicle_runtime.Success {
						gpuErr = fmt.Errorf("GPU NTT failed: %v", err)
						deviceSlice.Free()
						return
					}

					// Copy back
					hostSlice.CopyFromDevice(&deviceSlice)
					gpuTotal += time.Since(start)

					deviceSlice.Free()
				}
			})
			<-done

			if gpuErr != nil {
				t.Fatalf("%v", gpuErr)
			}

			gpuAvg := gpuTotal / time.Duration(runs)

			// Calculate speedup
			speedup := float64(cpuAvg) / float64(gpuAvg)

			fmt.Printf("Size 2^%d (%d elements):\n", logSize, size)
			fmt.Printf("  CPU FFT: %v\n", cpuAvg)
			fmt.Printf("  GPU NTT: %v (includes H2D + compute + D2H)\n", gpuAvg)
			fmt.Printf("  Speedup: %.2fx\n", speedup)
			if speedup > 1 {
				fmt.Printf("  Winner: GPU\n")
			} else {
				fmt.Printf("  Winner: CPU\n")
			}
			fmt.Println()

			t.Logf("CPU: %v, GPU: %v, Speedup: %.2fx", cpuAvg, gpuAvg, speedup)
		})
	}
}

// TestNTTBenchmarkComputeOnly benchmarks GPU NTT compute time only (no transfers in timing)
func TestNTTBenchmarkComputeOnly(t *testing.T) {
	// Load ICICLE backend
	loadErr := icicle_runtime.LoadBackendFromEnvOrDefault()
	if loadErr != icicle_runtime.Success {
		t.Fatalf("Failed to load ICICLE backend: %v", loadErr)
	}

	device := icicle_runtime.CreateDevice("CUDA", 0)

	sizes := []int{18, 19, 20, 21, 22, 23}
	runs := 10

	fmt.Println("=== GPU NTT PURE Compute Benchmark (no transfers in timing) ===")
	fmt.Println()

	for _, logSize := range sizes {
		size := 1 << logSize
		t.Run(fmt.Sprintf("Size_2^%d", logSize), func(t *testing.T) {
			// Generate random data
			data := make([]fr.Element, size)
			for i := range data {
				data[i].SetRandom()
			}

			// Create FFT domain
			domain := fft.NewDomain(uint64(size))

			// Benchmark CPU FFT
			cpuData := make([]fr.Element, size)
			var cpuTotal time.Duration
			for r := 0; r < runs; r++ {
				copy(cpuData, data)
				start := time.Now()
				domain.FFT(cpuData, fft.DIF)
				cpuTotal += time.Since(start)
			}
			cpuAvg := cpuTotal / time.Duration(runs)

			var gpuTotal time.Duration
			var gpuErr error

			done := make(chan bool, 1)
			icicle_runtime.RunOnDevice(&device, func(args ...any) {
				defer func() { done <- true }()

				// Initialize NTT domain
				rouIcicle := icicle_bn254.ScalarField{}
				rouBits := domain.Generator.Bits()
				limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
				rouIcicle.FromLimbs(limbs)

				cfg := icicle_core.GetDefaultNTTInitDomainConfig()
				initResult := icicle_ntt.InitDomain(rouIcicle, cfg)
				if initResult != icicle_runtime.Success {
					gpuErr = fmt.Errorf("Failed to init NTT domain: %v", initResult)
					return
				}
				defer icicle_ntt.ReleaseDomain()

				// Pre-allocate and copy data to GPU once (OUTSIDE timing)
				hostSlice := icicle_core.HostSliceFromElements(data)
				var inputDevice, outputDevice icicle_core.DeviceSlice
				hostSlice.CopyToDevice(&inputDevice, true)
				hostSlice.CopyToDevice(&outputDevice, true)

				// Warmup runs
				var cosetGen icicle_bn254.ScalarField
				nttCfg := icicle_core.GetDefaultNTTConfig(cosetGen)
				nttCfg.Ordering = icicle_core.KNR
				for w := 0; w < 3; w++ {
					icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfg, outputDevice)
				}

				// Force sync after warmup
				hostSlice.CopyFromDevice(&outputDevice)

				// Benchmark GPU NTT - PURE compute, NO transfers
				start := time.Now()
				for r := 0; r < runs; r++ {
					err := icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfg, outputDevice)
					if err != icicle_runtime.Success {
						gpuErr = fmt.Errorf("GPU NTT failed: %v", err)
						break
					}
				}
				// Final sync to ensure all NTTs complete
				hostSlice.CopyFromDevice(&outputDevice)
				gpuTotal = time.Since(start)

				inputDevice.Free()
				outputDevice.Free()
			})
			<-done

			if gpuErr != nil {
				t.Fatalf("%v", gpuErr)
			}

			gpuAvg := gpuTotal / time.Duration(runs)

			// Calculate speedup
			speedup := float64(cpuAvg) / float64(gpuAvg)

			fmt.Printf("Size 2^%d (%d elements):\n", logSize, size)
			fmt.Printf("  CPU FFT: %v\n", cpuAvg)
			fmt.Printf("  GPU NTT (pure compute): %v\n", gpuAvg)
			fmt.Printf("  Speedup: %.2fx\n", speedup)
			if speedup > 1 {
				fmt.Printf("  Winner: GPU\n")
			} else {
				fmt.Printf("  Winner: CPU\n")
			}
			fmt.Println()

			t.Logf("CPU: %v, GPU (compute): %v, Speedup: %.2fx", cpuAvg, gpuAvg, speedup)
		})
	}
}

// TestNTTBenchmarkAsyncVsSync tests async vs sync NTT
func TestNTTBenchmarkAsyncVsSync(t *testing.T) {
	// Load ICICLE backend
	loadErr := icicle_runtime.LoadBackendFromEnvOrDefault()
	if loadErr != icicle_runtime.Success {
		t.Fatalf("Failed to load ICICLE backend: %v", loadErr)
	}

	device := icicle_runtime.CreateDevice("CUDA", 0)

	sizes := []int{20, 21, 22, 23}
	runs := 5

	fmt.Println("=== GPU NTT Async vs Sync Comparison ===")
	fmt.Println()

	for _, logSize := range sizes {
		size := 1 << logSize
		fmt.Printf("Size 2^%d (%d elements):\n", logSize, size)

		// Generate random data
		data := make([]fr.Element, size)
		for i := range data {
			data[i].SetRandom()
		}

		// Create FFT domain
		domain := fft.NewDomain(uint64(size))

		done := make(chan bool, 1)
		icicle_runtime.RunOnDevice(&device, func(args ...any) {
			defer func() { done <- true }()

			// Initialize NTT domain
			rouIcicle := icicle_bn254.ScalarField{}
			rouBits := domain.Generator.Bits()
			limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
			rouIcicle.FromLimbs(limbs)

			cfg := icicle_core.GetDefaultNTTInitDomainConfig()
			initResult := icicle_ntt.InitDomain(rouIcicle, cfg)
			if initResult != icicle_runtime.Success {
				t.Fatalf("Failed to init NTT domain: %v", initResult)
			}
			defer icicle_ntt.ReleaseDomain()

			// Pre-allocate device memory
			hostSlice := icicle_core.HostSliceFromElements(data)
			var inputDevice, outputDevice icicle_core.DeviceSlice
			hostSlice.CopyToDevice(&inputDevice, true)
			hostSlice.CopyToDevice(&outputDevice, true)

			// Warmup
			var cosetGen icicle_bn254.ScalarField
			nttCfg := icicle_core.GetDefaultNTTConfig(cosetGen)
			nttCfg.Ordering = icicle_core.KNR
			icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfg, outputDevice)

			// Test sync mode
			{
				nttCfg := icicle_core.GetDefaultNTTConfig(cosetGen)
				nttCfg.Ordering = icicle_core.KNR
				nttCfg.IsAsync = false

				var total time.Duration
				for r := 0; r < runs; r++ {
					hostSlice.CopyToDevice(&inputDevice, true)
					start := time.Now()
					err := icicle_ntt.Ntt(inputDevice, icicle_core.KForward, &nttCfg, outputDevice)
					total += time.Since(start)
					if err != icicle_runtime.Success {
						t.Fatalf("GPU NTT sync failed: %v", err)
					}
				}
				avg := total / time.Duration(runs)
				fmt.Printf("  Sync: %v\n", avg)
			}

			inputDevice.Free()
			outputDevice.Free()
		})
		<-done
		fmt.Println()
	}
}
