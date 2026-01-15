// Copyright (c) Elliot Technologies, Inc.
// SPDX-License-Identifier: BUSL-1.1

// Official ICICLE MSM example from:
// https://dev.ingonyama.com/start/programmers_guide/go#multi-scalar-multiplication-msm-example

package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/core"
	"github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254"
	bn254Msm "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254/msm"
	"github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/runtime"
)

func main() {
	// Command line flags
	sizeLog := flag.Int("size", 18, "MSM size as power of 2 (default: 18 = 262,144 elements)")
	runs := flag.Int("runs", 3, "Number of runs")
	flag.Parse()

	size := 1 << *sizeLog

	fmt.Printf("MSM size: 2^%d = %d elements\n", *sizeLog, size)
	fmt.Printf("Runs: %d\n", *runs)
	fmt.Println("========================================")
	fmt.Println()

	// Load installed backends
	fmt.Println("Loading ICICLE backend...")
	err := runtime.LoadBackendFromEnvOrDefault()
	if err != runtime.Success {
		panic(fmt.Sprintf("Failed to load backend: %v", err))
	}

	// Trying to choose CUDA if available, or fallback to CPU otherwise
	deviceCuda := runtime.CreateDevice("CUDA", 0) // GPU-0
	if runtime.IsDeviceAvailable(&deviceCuda) {
		runtime.SetDevice(&deviceCuda)
		fmt.Println("✓ Using CUDA device (GPU-0)")
	} else {
		fmt.Println("⚠ CUDA not available, using CPU backend")
	}
	fmt.Println()

	// Setup inputs
	fmt.Printf("Generating %d random scalars and points...\n", size)
	startGen := time.Now()
	scalars := bn254.GenerateScalars(size)
	// scalars[0].FromUint32(1)
	scalars[0].One()
	points := bn254.GenerateAffinePoints(size)
	// points[0].Zero()
	points[0].X.One()
	points[0].Y.One()

	genTime := time.Since(startGen)
	fmt.Printf("✓ Generated in %v\n", genTime)
	fmt.Println()
	// for i := 1; i < size; i++ {
	// 	scalars[i].Zero()
	// 	points[i].X.Zero()
	// 	points[i].Y.Zero()
	// }
	// fmt.Printf("Scalars: %v\n", scalars)
	// fmt.Printf("Points: %v\n", points)

	// Warm-up run
	fmt.Println("Running warm-up...")
	{
		var scalarsDevice core.DeviceSlice
		scalars.CopyToDevice(&scalarsDevice, true)
		cfgBn254 := core.GetDefaultMSMConfig()
		result := make(core.HostSlice[bn254.Projective], 1)
		bn254Msm.Msm(scalarsDevice, points, &cfgBn254, result)
		scalarsDevice.Free()
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

		// (optional) copy scalars to device memory explicitly
		copyStart := time.Now()
		var scalarsDevice core.DeviceSlice
		scalars.CopyToDevice(&scalarsDevice, true)
		var pointsDevice core.DeviceSlice
		points.CopyToDevice(&pointsDevice, true)
		copyDuration := time.Since(copyStart)

		// MSM configuration
		cfgBn254 := core.GetDefaultMSMConfig()

		// allocate memory for the result
		result := make(core.HostSlice[bn254.Projective], 1)

		// execute bn254 MSM on device
		msmStart := time.Now()
		// err := bn254Msm.Msm(scalarsDevice, points, &cfgBn254, result)
		err := bn254Msm.Msm(scalarsDevice, pointsDevice, &cfgBn254, result)
		msmDuration := time.Since(msmStart)
		// fmt.Printf("msm result: %v\n", result[0])

		// Check for errors
		if err != runtime.Success {
			errorString := fmt.Sprint("bn254 Msm failed: ", err)
			panic(errorString)
		}

		// free explicitly allocated device memory
		scalarsDevice.Free()

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
	fmt.Println("Results")
	fmt.Println("========================================")
	fmt.Printf("Average total time:  %v\n", avgTotal)
	fmt.Printf("Average copy time:   %v (%.1f%%)\n", avgCopy, 100*float64(avgCopy)/float64(avgTotal))
	fmt.Printf("Average MSM time:    %v (%.1f%%)\n", avgMSM, 100*float64(avgMSM)/float64(avgTotal))
	fmt.Println()
	fmt.Printf("Throughput: %.2f MSMs/second\n", float64(*runs)/totalTime.Seconds())
	fmt.Println("========================================")
}
