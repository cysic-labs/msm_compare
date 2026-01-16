//go:build icicle

// ICICLE v2 MSM benchmark example
// Adapted from v3 example to use ICICLE v2 API

package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/ingonyama-zk/icicle/v2/wrappers/golang/core"
	cr "github.com/ingonyama-zk/icicle/v2/wrappers/golang/cuda_runtime"
	"github.com/ingonyama-zk/icicle/v2/wrappers/golang/curves/bn254"
	bn254Msm "github.com/ingonyama-zk/icicle/v2/wrappers/golang/curves/bn254/msm"
)

func main() {
	// Command line flags
	sizeLog := flag.Int("size", 18, "MSM size as power of 2 (default: 18 = 262,144 elements)")
	runs := flag.Int("runs", 3, "Number of runs")
	flag.Parse()

	size := 1 << *sizeLog

	fmt.Printf("ICICLE v2 MSM Benchmark\n")
	fmt.Printf("MSM size: 2^%d = %d elements\n", *sizeLog, size)
	fmt.Printf("Runs: %d\n", *runs)
	fmt.Println("========================================")
	fmt.Println()

	// Check CUDA device
	deviceCount, err := cr.GetDeviceCount()
	if err != cr.CudaSuccess || deviceCount == 0 {
		panic("No CUDA devices available")
	}
	fmt.Printf("✓ Found %d CUDA device(s)\n", deviceCount)

	// Set device 0
	err = cr.SetDevice(0)
	if err != cr.CudaSuccess {
		panic(fmt.Sprintf("Failed to set device: %v", err))
	}
	fmt.Println("✓ Using CUDA device 0")
	fmt.Println()

	// Setup inputs
	fmt.Printf("Generating %d random scalars and points...\n", size)
	startGen := time.Now()
	scalars := bn254.GenerateScalars(size)
	points := bn254.GenerateAffinePoints(size)
	genTime := time.Since(startGen)
	fmt.Printf("✓ Generated in %v\n", genTime)
	fmt.Println()

	// Create CUDA stream for async operations
	stream, err := cr.CreateStream()
	if err != cr.CudaSuccess {
		panic(fmt.Sprintf("Failed to create stream: %v", err))
	}
	defer cr.DestroyStream(&stream)

	// Warm-up run
	fmt.Println("Running warm-up...")
	{
		cfg := core.GetDefaultMSMConfig()
		var msmResult core.DeviceSlice
		var projective bn254.Projective
		msmResult.Malloc(projective.Size(), projective.Size())
		defer msmResult.Free()

		e := bn254Msm.Msm(scalars, points, &cfg, msmResult)
		if e != cr.CudaSuccess {
			panic(fmt.Sprintf("Warm-up MSM failed: %v", e))
		}
	}
	fmt.Println("✓ Warm-up complete")
	fmt.Println()

	// Benchmark runs
	fmt.Printf("Running %d benchmark iterations...\n", *runs)
	var totalTime time.Duration
	var copyTime, msmTime time.Duration

	for i := 0; i < *runs; i++ {
		fmt.Printf("  Run %d/%d: ", i+1, *runs)

		runStart := time.Now()

		// Copy scalars and points to device
		copyStart := time.Now()

		var scalarsDevice core.DeviceSlice
		scalars.CopyToDevice(&scalarsDevice, true)

		var pointsDevice core.DeviceSlice
		points.CopyToDevice(&pointsDevice, true)

		copyDuration := time.Since(copyStart)

		// MSM configuration
		cfg := core.GetDefaultMSMConfig()

		// Allocate result on device
		var msmResult core.DeviceSlice
		var projective bn254.Projective
		msmResult.Malloc(projective.Size(), projective.Size())

		// Execute MSM
		msmStart := time.Now()
		e := bn254Msm.Msm(scalarsDevice, pointsDevice, &cfg, msmResult)
		if e != cr.CudaSuccess {
			panic(fmt.Sprintf("MSM failed: %v", e))
		}
		msmDuration := time.Since(msmStart)

		// Copy result back to host
		resultHost := make(core.HostSlice[bn254.Projective], 1)
		resultHost.CopyFromDevice(&msmResult)

		// Free device memory
		scalarsDevice.Free()
		pointsDevice.Free()
		msmResult.Free()

		runDuration := time.Since(runStart)
		totalTime += runDuration
		copyTime += copyDuration
		msmTime += msmDuration

		fmt.Printf("Total: %v (Copy: %v, MSM: %v)\n", runDuration, copyDuration, msmDuration)
	}

	avgTotal := totalTime / time.Duration(*runs)
	avgCopy := copyTime / time.Duration(*runs)
	avgMSM := msmTime / time.Duration(*runs)

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Results (ICICLE v2)")
	fmt.Println("========================================")
	fmt.Printf("Average total time:  %v\n", avgTotal)
	fmt.Printf("Average copy time:   %v (%.1f%%)\n", avgCopy, 100*float64(avgCopy)/float64(avgTotal))
	fmt.Printf("Average MSM time:    %v (%.1f%%)\n", avgMSM, 100*float64(avgMSM)/float64(avgTotal))
	fmt.Println()
	fmt.Printf("Throughput: %.2f MSMs/second\n", float64(*runs)/totalTime.Seconds())
	fmt.Println("========================================")
}
