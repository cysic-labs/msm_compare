//go:build icicle

// ICICLE v2 NTT benchmark
package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	bn254Fft "github.com/consensys/gnark-crypto/ecc/bn254/fr/fft"

	"github.com/ingonyama-zk/icicle/v2/wrappers/golang/core"
	cr "github.com/ingonyama-zk/icicle/v2/wrappers/golang/cuda_runtime"
	"github.com/ingonyama-zk/icicle/v2/wrappers/golang/curves/bn254"
	bn254Ntt "github.com/ingonyama-zk/icicle/v2/wrappers/golang/curves/bn254/ntt"
)

func main() {
	sizeLog := flag.Int("size", 18, "NTT size as power of 2")
	runs := flag.Int("runs", 3, "Number of runs")
	flag.Parse()

	size := 1 << *sizeLog

	fmt.Printf("=== NTT Benchmark: CPU FFT vs ICICLE v2 GPU NTT ===\n")
	fmt.Printf("Size: 2^%d = %d elements\n", *sizeLog, size)
	fmt.Printf("Runs: %d\n\n", *runs)

	// Check CUDA device
	deviceCount, err := cr.GetDeviceCount()
	if err != cr.CudaSuccess || deviceCount == 0 {
		panic("No CUDA devices available")
	}
	cr.SetDevice(0)
	fmt.Printf("Using CUDA device 0\n\n")

	// Create stream
	stream, _ := cr.CreateStream()
	defer cr.DestroyStream(&stream)

	// Generate data
	fmt.Println("Generating data...")
	frData := make([]fr.Element, size)
	for i := range frData {
		frData[i].SetRandom()
	}

	// CPU FFT benchmark
	fmt.Println("\n=== CPU FFT (gnark-crypto) ===")
	domain := bn254Fft.NewDomain(uint64(size))
	var cpuTotal time.Duration
	for r := 0; r < *runs; r++ {
		cpuData := make([]fr.Element, size)
		copy(cpuData, frData)
		start := time.Now()
		domain.FFT(cpuData, bn254Fft.DIF)
		cpuTotal += time.Since(start)
		fmt.Printf("  Run %d: %v\n", r+1, time.Since(start))
	}
	cpuAvg := cpuTotal / time.Duration(*runs)
	fmt.Printf("Average: %v\n", cpuAvg)

	// ICICLE v2 GPU NTT benchmark
	fmt.Println("\n=== GPU NTT (ICICLE v2) ===")

	// Generate ICICLE scalars
	scalars := bn254.GenerateScalars(size)

	// Initialize NTT domain
	rouMont, _ := bn254Fft.Generator(uint64(size))
	rou := rouMont.Bits()
	rouIcicle := bn254.ScalarField{}
	limbs := core.ConvertUint64ArrToUint32Arr(rou[:])
	rouIcicle.FromLimbs(limbs)

	cfg := bn254Ntt.GetDefaultNttConfig()
	cfg.Ctx.Stream = &stream
	cfg.IsAsync = true

	bn254Ntt.InitDomain(rouIcicle, cfg.Ctx, false)
	defer bn254Ntt.ReleaseDomain(cfg.Ctx)

	// Warmup
	var nttResult core.DeviceSlice
	nttResult.MallocAsync(size*scalars.SizeOfElement(), scalars.SizeOfElement(), stream)
	defer nttResult.FreeAsync(stream)

	bn254Ntt.Ntt(scalars, core.KForward, &cfg, nttResult)
	cr.SynchronizeStream(&stream)

	// Benchmark
	var gpuTotal time.Duration
	for r := 0; r < *runs; r++ {
		start := time.Now()
		bn254Ntt.Ntt(scalars, core.KForward, &cfg, nttResult)
		cr.SynchronizeStream(&stream)
		gpuTotal += time.Since(start)
		fmt.Printf("  Run %d: %v\n", r+1, time.Since(start))
	}
	gpuAvg := gpuTotal / time.Duration(*runs)
	fmt.Printf("Average: %v\n", gpuAvg)

	// Summary
	fmt.Println("\n=== Summary ===")
	fmt.Printf("CPU FFT: %v\n", cpuAvg)
	fmt.Printf("GPU NTT: %v\n", gpuAvg)
	speedup := float64(cpuAvg) / float64(gpuAvg)
	if speedup > 1 {
		fmt.Printf("GPU is %.2fx faster\n", speedup)
	} else {
		fmt.Printf("CPU is %.2fx faster\n", 1/speedup)
	}
}
