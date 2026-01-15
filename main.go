// Copyright (c) Elliot Technologies, Inc.
// SPDX-License-Identifier: BUSL-1.1

//go:build icicle

package main

import (
	"flag"
	"fmt"
	"math/big"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fp"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"

	icicle_core "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/core"
	icicle_bn254 "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254"
	icicle_msm "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254/msm"
	icicle_runtime "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/runtime"
)

// MSM test configuration
type MSMTest struct {
	Degree      int
	Size        int
	Description string
}

// Timing breakdown for GPU MSM
type GPUTimingBreakdown struct {
	BasesH2D          time.Duration
	ScalarsH2D        time.Duration
	BasesMontgomery   time.Duration
	ScalarsMontgomery time.Duration
	MSMCompute        time.Duration
	ResultD2H         time.Duration
	Total             time.Duration
}

func main() {
	minDegree := flag.Int("min", 7, "Minimum degree (2^min)")
	maxDegree := flag.Int("max", 20, "Maximum degree (2^max)")
	runs := flag.Int("runs", 3, "Number of runs per test")
	detailed := flag.Bool("detailed", false, "Show detailed timing breakdown")
	flag.Parse()

	fmt.Println("========================================")
	fmt.Println("MSM Performance Comparison: CPU vs GPU")
	fmt.Println("========================================")
	fmt.Printf("Degree range: 2^%d to 2^%d\n", *minDegree, *maxDegree)
	fmt.Printf("Runs per test: %d\n", *runs)
	if *detailed {
		fmt.Println("Mode: Detailed timing breakdown")
	}
	fmt.Println("========================================")
	fmt.Println()

	// Initialize ICICLE
	fmt.Println("Initializing ICICLE backend...")
	err := icicle_runtime.LoadBackendFromEnvOrDefault()
	if err != icicle_runtime.Success {
		fmt.Printf("Failed to load ICICLE backend: %v\n", err)
		return
	}
	device := icicle_runtime.CreateDevice("CUDA", 0)
	icicle_runtime.SetDevice(&device)
	fmt.Println("✓ ICICLE backend initialized")
	fmt.Println()

	// Generate test cases
	var tests []MSMTest
	for degree := *minDegree; degree <= *maxDegree; degree++ {
		size := 1 << degree
		tests = append(tests, MSMTest{
			Degree:      degree,
			Size:        size,
			Description: fmt.Sprintf("2^%d", degree),
		})
	}

	// Results table
	fmt.Println("========================================")
	fmt.Println("Results")
	fmt.Println("========================================")
	fmt.Printf("%-10s %-15s %-15s %-15s %-15s\n", "Degree", "Size", "CPU Time", "GPU Time", "Speedup")
	fmt.Println("--------------------------------------------------------------------------------")

	for _, test := range tests {
		fmt.Printf("Testing %s (%d elements)...\n", test.Description, test.Size)
		cpuTime, gpuTime, breakdown := runMSMTest(test, *runs, *detailed)
		speedup := float64(cpuTime) / float64(gpuTime)

		var speedupStr string
		if speedup > 1.0 {
			speedupStr = fmt.Sprintf("%.2fx GPU", speedup)
		} else {
			speedupStr = fmt.Sprintf("%.2fx CPU", 1.0/speedup)
		}

		fmt.Printf("%-10s %-15d %-15s %-15s %-15s\n",
			test.Description,
			test.Size,
			formatDuration(cpuTime),
			formatDuration(gpuTime),
			speedupStr)

		if *detailed && breakdown != nil {
			fmt.Println("  GPU Timing Breakdown:")
			fmt.Printf("    Bases H2D:          %s (%.1f%%)\n", formatDuration(breakdown.BasesH2D), 100*float64(breakdown.BasesH2D)/float64(breakdown.Total))
			fmt.Printf("    Scalars H2D:        %s (%.1f%%)\n", formatDuration(breakdown.ScalarsH2D), 100*float64(breakdown.ScalarsH2D)/float64(breakdown.Total))
			fmt.Printf("    Bases Montgomery:   %s (%.1f%%)\n", formatDuration(breakdown.BasesMontgomery), 100*float64(breakdown.BasesMontgomery)/float64(breakdown.Total))
			fmt.Printf("    Scalars Montgomery: %s (%.1f%%)\n", formatDuration(breakdown.ScalarsMontgomery), 100*float64(breakdown.ScalarsMontgomery)/float64(breakdown.Total))
			fmt.Printf("    MSM Compute:        %s (%.1f%%)\n", formatDuration(breakdown.MSMCompute), 100*float64(breakdown.MSMCompute)/float64(breakdown.Total))
			fmt.Printf("    Result D2H:         %s (%.1f%%)\n", formatDuration(breakdown.ResultD2H), 100*float64(breakdown.ResultD2H)/float64(breakdown.Total))
			fmt.Printf("    Total:              %s\n", formatDuration(breakdown.Total))
			fmt.Println()
		}
	}

	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("✅ MSM comparison complete!")
	// fmt.Println()
	// fmt.Println("Key observations:")
	// fmt.Println("  - Small sizes (2^7-2^13): CPU is faster (lower overhead)")
	// fmt.Println("  - Medium sizes (2^14): CPU and GPU are roughly equal")
	// fmt.Println("  - Large sizes (2^15+): GPU becomes faster")
	// fmt.Println("  - Very large sizes (2^20+): GPU significantly faster")
	// fmt.Println()
	// fmt.Println("Note: GPU times include all overhead (H2D copy, Montgomery conversion, MSM, D2H copy)")
	// fmt.Println("      In real-world usage (like PLONK proving), bases are pre-loaded to GPU,")
	// fmt.Println("      reducing per-MSM overhead significantly.")
	// if *detailed {
	// 	fmt.Println()
	// 	fmt.Println("💡 Tip: Montgomery conversions are a significant overhead!")
	// 	fmt.Println("   In PLONK proving, bases (SRS) are pre-loaded and pre-converted,")
	// 	fmt.Println("   so only scalar conversion is needed per MSM.")
	// }
}

func runMSMTest(test MSMTest, runs int, detailed bool) (time.Duration, time.Duration, *GPUTimingBreakdown) {
	// Generate random scalars and bases
	scalars := make([]fr.Element, test.Size)
	bases := make([]bn254.G1Affine, test.Size)

	// Random scalars
	for i := 0; i < test.Size; i++ {
		scalars[i].SetRandom()
	}

	// Random bases (using generator for simplicity)
	_, _, g1Gen, _ := bn254.Generators()
	for i := 0; i < test.Size; i++ {
		var scalar fr.Element
		scalar.SetRandom()
		bases[i].ScalarMultiplication(&g1Gen, scalar.BigInt(new(big.Int)))
	}

	// fmt.Printf("finish prepare bases of size: %v\n", len(bases))
	// fmt.Printf("finish prepare scalars of size: %v\n", len(scalars))

	// Warm up
	_ = runCPUMSM(bases, scalars)
	_, _ = runGPUMSM(bases, scalars, false)

	// CPU benchmark
	// fmt.Println("begin cpu MSM")
	var cpuTotal time.Duration
	var cpu_res bn254.G1Affine
	for i := 0; i < runs; i++ {
		start := time.Now()
		cpu_res = runCPUMSM(bases, scalars)
		cpuTotal += time.Since(start)
	}
	// fmt.Println("  end cpu MSM")
	cpuAvg := cpuTotal / time.Duration(runs)

	// GPU benchmark
	var gpuTotal time.Duration
	var breakdown *GPUTimingBreakdown
	var gpu_res bn254.G1Affine
	var bd *GPUTimingBreakdown
	for i := 0; i < runs; i++ {
		start := time.Now()
		gpu_res, bd = runGPUMSM(bases, scalars, detailed && i == 0)
		gpuTotal += time.Since(start)
		if detailed && i == 0 {
			breakdown = bd
		}
	}
	gpuAvg := gpuTotal / time.Duration(runs)

	_ = gpu_res
	_ = cpu_res
	// fmt.Printf("cpu res: %v\n", cpu_res)
	// fmt.Printf("gpu res: %v\n", gpu_res)
	// fmt.Printf("gpu bd : %v\n", bd)

	return cpuAvg, gpuAvg, breakdown
}

func runCPUMSM(bases []bn254.G1Affine, scalars []fr.Element) bn254.G1Affine {
	var result bn254.G1Affine
	result.MultiExp(bases, scalars, ecc.MultiExpConfig{})
	return result
}

func runGPUMSM(bases []bn254.G1Affine, scalars []fr.Element, measureTiming bool) (bn254.G1Affine, *GPUTimingBreakdown) {
	var breakdown *GPUTimingBreakdown
	if measureTiming {
		breakdown = &GPUTimingBreakdown{}
	}

	totalStart := time.Now()

	// Prepare output buffer
	resultIcicle := make(icicle_core.HostSlice[icicle_bn254.Projective], 1)

	// Allocate device memory
	var basesDevice, scalarsDevice icicle_core.DeviceSlice

	// Copy bases to device
	if measureTiming {
		start := time.Now()
		basesHost := (icicle_core.HostSlice[bn254.G1Affine])(bases)
		basesHost.CopyToDevice(&basesDevice, true)
		breakdown.BasesH2D = time.Since(start)
	} else {
		basesHost := (icicle_core.HostSlice[bn254.G1Affine])(bases)
		basesHost.CopyToDevice(&basesDevice, true)
	}

	// Copy scalars to device
	if measureTiming {
		start := time.Now()
		scalarsHost := icicle_core.HostSliceFromElements(scalars)
		scalarsHost.CopyToDevice(&scalarsDevice, true)
		breakdown.ScalarsH2D = time.Since(start)
	} else {
		scalarsHost := icicle_core.HostSliceFromElements(scalars)
		scalarsHost.CopyToDevice(&scalarsDevice, true)
	}

	// Convert bases from Montgomery form
	if measureTiming {
		start := time.Now()
		icicle_bn254.AffineFromMontgomery(basesDevice)
		breakdown.BasesMontgomery = time.Since(start)
	} else {
		icicle_bn254.AffineFromMontgomery(basesDevice)
	}

	// Convert scalars from Montgomery form
	if measureTiming {
		start := time.Now()
		icicle_bn254.FromMontgomery(scalarsDevice)
		breakdown.ScalarsMontgomery = time.Since(start)
	} else {
		icicle_bn254.FromMontgomery(scalarsDevice)
	}

	// Configure and run MSM
	if measureTiming {
		start := time.Now()
		cfg := icicle_msm.GetDefaultMSMConfig()
		icicle_msm.Msm(scalarsDevice, basesDevice, &cfg, resultIcicle)
		breakdown.MSMCompute = time.Since(start)
	} else {
		cfg := icicle_msm.GetDefaultMSMConfig()
		icicle_msm.Msm(scalarsDevice, basesDevice, &cfg, resultIcicle)
	}

	// Result is already in host memory (resultIcicle is HostSlice)
	// D2H happens implicitly in MSM call
	if measureTiming {
		breakdown.ResultD2H = 0 // Already included in MSM time
	}

	// Free device memory
	basesDevice.Free()
	scalarsDevice.Free()

	if measureTiming {
		breakdown.Total = time.Since(totalStart)
	}

	// Convert result back to gnark format
	return projectiveToGnarkAffine(resultIcicle[0]), breakdown
}

func projectiveToGnarkAffine(p icicle_bn254.Projective) bn254.G1Affine {
	px, _ := fp.LittleEndian.Element((*[fp.Bytes]byte)(p.X.ToBytesLittleEndian()))
	py, _ := fp.LittleEndian.Element((*[fp.Bytes]byte)(p.Y.ToBytesLittleEndian()))
	pz, _ := fp.LittleEndian.Element((*[fp.Bytes]byte)(p.Z.ToBytesLittleEndian()))

	var x, y, zInv fp.Element
	zInv.Inverse(&pz)
	x.Mul(&px, &zInv)
	y.Mul(&py, &zInv)

	return bn254.G1Affine{X: x, Y: y}
}

func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%d ns", d.Nanoseconds())
	} else if d < time.Millisecond {
		return fmt.Sprintf("%.2f μs", float64(d.Microseconds()))
	} else if d < time.Second {
		return fmt.Sprintf("%.2f ms", float64(d.Milliseconds()))
	} else {
		return fmt.Sprintf("%.2f s", d.Seconds())
	}
}
