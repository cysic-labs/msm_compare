//go:build icicle

package main

import (
	"fmt"
	"time"

	icicle_bn254 "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254"
	icicle_core "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/core"
	icicle_msm "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254/msm"
	icicle_ntt "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254/ntt"
	icicle_runtime "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/runtime"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr/fft"
)

func main() {
	fmt.Println("=== ICICLE Backend Debug with Timing ===")
	fmt.Println()

	// Load backend
	fmt.Println("Loading backend...")
	err := icicle_runtime.LoadBackendFromEnvOrDefault()
	if err != icicle_runtime.Success {
		panic(fmt.Sprintf("Failed to load backend: %v", err))
	}

	// List registered devices
	devices, _ := icicle_runtime.GetRegisteredDevices()
	fmt.Printf("Registered devices: %v\n", devices)

	// Set CUDA device
	cudaDevice := icicle_runtime.CreateDevice("CUDA", 0)
	icicle_runtime.SetDevice(&cudaDevice)
	activeDevice, _ := icicle_runtime.GetActiveDevice()
	fmt.Printf("Active device: %s\n\n", activeDevice.GetDeviceType())

	size := 1 << 18

	// Generate data
	scalars := icicle_bn254.GenerateScalars(size)
	points := icicle_bn254.GenerateAffinePoints(size)

	// Test MSM
	fmt.Printf("=== MSM Test (size: %d) ===\n", size)
	var scalarsDevice, pointsDevice icicle_core.DeviceSlice
	scalars.CopyToDevice(&scalarsDevice, true)
	points.CopyToDevice(&pointsDevice, true)
	msmCfg := icicle_core.GetDefaultMSMConfig()
	msmResult := make(icicle_core.HostSlice[icicle_bn254.Projective], 1)

	// Warmup
	icicle_msm.Msm(scalarsDevice, pointsDevice, &msmCfg, msmResult)

	// Timed run
	start := time.Now()
	icicle_msm.Msm(scalarsDevice, pointsDevice, &msmCfg, msmResult)
	msmTime := time.Since(start)
	fmt.Printf("MSM time: %v\n", msmTime)

	// Async test
	stream, _ := icicle_runtime.CreateStream()
	msmCfg.StreamHandle = stream
	msmCfg.IsAsync = true
	start = time.Now()
	icicle_msm.Msm(scalarsDevice, pointsDevice, &msmCfg, msmResult)
	asyncCall := time.Since(start)
	start = time.Now()
	icicle_runtime.SynchronizeStream(stream)
	syncTime := time.Since(start)
	fmt.Printf("Async MSM: call=%v, sync=%v\n", asyncCall, syncTime)
	if asyncCall < 5*time.Millisecond {
		fmt.Println("✓ GPU MSM confirmed (async returns quickly)")
	} else {
		fmt.Println("✗ CPU MSM (async blocks)")
	}
	icicle_runtime.DestroyStream(stream)
	scalarsDevice.Free()
	pointsDevice.Free()

	// Test NTT
	fmt.Printf("\n=== NTT Test (size: %d) ===\n", size)

	// Init domain
	domain := fft.NewDomain(uint64(size))
	rouIcicle := icicle_bn254.ScalarField{}
	rouBits := domain.Generator.Bits()
	limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
	rouIcicle.FromLimbs(limbs)

	initCfg := icicle_core.GetDefaultNTTInitDomainConfig()
	icicle_ntt.InitDomain(rouIcicle, initCfg)
	defer icicle_ntt.ReleaseDomain()

	// Allocate device memory
	nttScalars := icicle_bn254.GenerateScalars(size)
	var nttInputDevice, nttOutputDevice icicle_core.DeviceSlice
	nttScalars.CopyToDevice(&nttInputDevice, true)
	nttScalars.CopyToDevice(&nttOutputDevice, true)
	nttCfg := icicle_ntt.GetDefaultNttConfig()

	// Warmup
	icicle_ntt.Ntt(nttInputDevice, icicle_core.KForward, &nttCfg, nttOutputDevice)

	// Timed run
	nttScalars.CopyToDevice(&nttInputDevice, true)
	start = time.Now()
	icicle_ntt.Ntt(nttInputDevice, icicle_core.KForward, &nttCfg, nttOutputDevice)
	nttTime := time.Since(start)
	fmt.Printf("NTT time: %v\n", nttTime)

	// Async test
	stream2, _ := icicle_runtime.CreateStream()
	nttCfg.StreamHandle = stream2
	nttCfg.IsAsync = true
	nttScalars.CopyToDevice(&nttInputDevice, true)
	start = time.Now()
	icicle_ntt.Ntt(nttInputDevice, icicle_core.KForward, &nttCfg, nttOutputDevice)
	asyncCall = time.Since(start)
	start = time.Now()
	icicle_runtime.SynchronizeStream(stream2)
	syncTime = time.Since(start)
	fmt.Printf("Async NTT: call=%v, sync=%v\n", asyncCall, syncTime)
	if asyncCall < 5*time.Millisecond {
		fmt.Println("✓ GPU NTT confirmed (async returns quickly)")
	} else {
		fmt.Println("✗ CPU NTT (async blocks)")
	}
	icicle_runtime.DestroyStream(stream2)
	nttInputDevice.Free()
	nttOutputDevice.Free()

	fmt.Println("\n=== Summary ===")
	fmt.Printf("MSM: %v\n", msmTime)
	fmt.Printf("NTT: %v\n", nttTime)
	if msmTime < 10*time.Millisecond && nttTime > 30*time.Millisecond {
		fmt.Println("\nConclusion: MSM uses GPU, NTT falls back to CPU")
	}
}
