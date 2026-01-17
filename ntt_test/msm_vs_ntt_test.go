//go:build icicle

package ntt_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr/fft"

	icicle_bn254 "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254"
	icicle_core "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/core"
	icicle_msm "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254/msm"
	icicle_ntt "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/curves/bn254/ntt"
	icicle_runtime "github.com/ingonyama-zk/icicle-gnark/v3/wrappers/golang/runtime"
)

func init() {
	os.Setenv("ICICLE_BACKEND_INSTALL_DIR", "/home/cysic/coding/plonky/msm_compare/lib/backend")
}

// TestMSMvsNTTAsync compares async behavior of MSM (working) vs NTT (not working)
func TestMSMvsNTTAsync(t *testing.T) {
	err := icicle_runtime.LoadBackendFromEnvOrDefault()
	if err != icicle_runtime.Success {
		t.Fatalf("Failed to load backend: %v", err)
	}

	device := icicle_runtime.CreateDevice("CUDA", 0)
	icicle_runtime.SetDevice(&device)

	size := 1 << 18 // 2^18
	fmt.Printf("=== MSM vs NTT Async Behavior (size: %d) ===\n\n", size)

	// Generate data
	scalars := icicle_bn254.GenerateScalars(size)
	points := icicle_bn254.GenerateAffinePoints(size)

	// Create stream
	stream, _ := icicle_runtime.CreateStream()
	defer icicle_runtime.DestroyStream(stream)

	// ========== TEST MSM ==========
	fmt.Println("=== MSM Test ===")

	// Copy data to device
	var scalarsDevice, pointsDevice icicle_core.DeviceSlice
	scalars.CopyToDevice(&scalarsDevice, true)
	points.CopyToDevice(&pointsDevice, true)

	// Configure MSM
	msmCfg := icicle_core.GetDefaultMSMConfig()
	msmCfg.StreamHandle = stream
	msmCfg.IsAsync = true

	// Warmup
	result := make(icicle_core.HostSlice[icicle_bn254.Projective], 1)
	for w := 0; w < 3; w++ {
		icicle_msm.Msm(scalarsDevice, pointsDevice, &msmCfg, result)
	}
	icicle_runtime.SynchronizeStream(stream)

	// Test async
	fmt.Println("Async MSM:")
	asyncStart := time.Now()
	icicle_msm.Msm(scalarsDevice, pointsDevice, &msmCfg, result)
	asyncCallTime := time.Since(asyncStart)
	fmt.Printf("  MSM call time: %v\n", asyncCallTime)

	syncStart := time.Now()
	icicle_runtime.SynchronizeStream(stream)
	syncTime := time.Since(syncStart)
	fmt.Printf("  Stream sync time: %v\n", syncTime)
	fmt.Printf("  Total: %v\n", asyncCallTime+syncTime)

	if asyncCallTime < 5*time.Millisecond {
		fmt.Println("  ✓ GPU MSM: async call returns quickly")
	} else {
		fmt.Println("  ✗ CPU MSM: async call blocks")
	}

	scalarsDevice.Free()
	pointsDevice.Free()

	// ========== TEST NTT ==========
	fmt.Println("\n=== NTT Test ===")

	// Initialize NTT domain
	domain := fft.NewDomain(uint64(size))
	rouIcicle := icicle_bn254.ScalarField{}
	rouBits := domain.Generator.Bits()
	limbs := icicle_core.ConvertUint64ArrToUint32Arr(rouBits[:])
	rouIcicle.FromLimbs(limbs)

	initCfg := icicle_core.GetDefaultNTTInitDomainConfig()
	icicle_ntt.InitDomain(rouIcicle, initCfg)
	defer icicle_ntt.ReleaseDomain()

	// Copy data
	nttScalars := icicle_bn254.GenerateScalars(size)
	var nttInputDevice, nttOutputDevice icicle_core.DeviceSlice
	nttScalars.CopyToDevice(&nttInputDevice, true)
	nttScalars.CopyToDevice(&nttOutputDevice, true)

	// Configure NTT
	nttCfg := icicle_ntt.GetDefaultNttConfig()
	nttCfg.StreamHandle = stream
	nttCfg.IsAsync = true

	// Warmup
	for w := 0; w < 3; w++ {
		icicle_ntt.Ntt(nttInputDevice, icicle_core.KForward, &nttCfg, nttOutputDevice)
	}
	icicle_runtime.SynchronizeStream(stream)

	// Test async
	fmt.Println("Async NTT:")
	nttScalars.CopyToDevice(&nttInputDevice, true) // Reset
	asyncStart = time.Now()
	icicle_ntt.Ntt(nttInputDevice, icicle_core.KForward, &nttCfg, nttOutputDevice)
	asyncCallTime = time.Since(asyncStart)
	fmt.Printf("  NTT call time: %v\n", asyncCallTime)

	syncStart = time.Now()
	icicle_runtime.SynchronizeStream(stream)
	syncTime = time.Since(syncStart)
	fmt.Printf("  Stream sync time: %v\n", syncTime)
	fmt.Printf("  Total: %v\n", asyncCallTime+syncTime)

	if asyncCallTime < 5*time.Millisecond {
		fmt.Println("  ✓ GPU NTT: async call returns quickly")
	} else {
		fmt.Println("  ✗ CPU NTT: async call blocks")
	}

	nttInputDevice.Free()
	nttOutputDevice.Free()

	fmt.Println("\n=== Summary ===")
	fmt.Println("MSM and NTT should both return quickly for async calls if using GPU.")
	fmt.Println("If MSM is quick but NTT blocks, NTT is falling back to CPU.")
}
