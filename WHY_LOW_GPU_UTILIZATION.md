# Why is GPU Utilization So Low? (17%)

## Investigation Summary

After extensive testing and profiling, here's what we know about the low GPU utilization issue:

## The Core Problem

The ICICLE MSM kernel for BN254 on RTX 4090 has **extremely low GPU utilization (~17% SM usage)**, which causes:
1. GPU power management to randomly switch between P0 and P5 states
2. Inconsistent performance (17ms to 7 seconds)
3. Even with locked clocks, performance can still be slow

## Why is Utilization So Low?

### 1. **Insufficient Parallelism / Poor Kernel Occupancy**

The MSM algorithm uses the "bucket method" (Pippenger's algorithm):
- Divides scalars into windows of size `c` bits
- Creates 2^c buckets per window
- Accumulates points into buckets
- Final reduction across buckets

**The problem**: For BN254 with 254-bit scalars and default `c` value, the kernel may not generate enough parallel work to saturate 16,384 CUDA cores on RTX 4090.

### 2. **Memory-Bound Operations**

From the library symbols, we can see:
- `MontgomeryKernel` - Montgomery form conversions
- `msm_cuda` - Main MSM computation

These operations involve:
- Random memory access patterns (bucket accumulation)
- Atomic operations (multiple threads writing to same bucket)
- High memory bandwidth requirements

**The RTX 4090 has**:
- 16,384 CUDA cores
- 1008 GB/s memory bandwidth
- 24 GB GDDR6X memory

If the MSM kernel is memory-bound rather than compute-bound, the GPU cores will be idle waiting for memory, resulting in low SM utilization.

### 3. **Serialization Due to Atomics**

The bucket accumulation phase requires atomic operations when multiple points map to the same bucket. This creates serialization:
- Thread 1 wants to add to bucket[42]
- Thread 2 also wants to add to bucket[42]
- Thread 2 must wait for Thread 1 to complete
- Result: Threads are idle, low utilization

### 4. **Small Kernel Launch Configuration**

The ICICLE library may be using conservative kernel launch parameters:
- Too few thread blocks
- Too few threads per block
- Not enough warps to hide latency

This would leave many SMs idle.

## Why Clock Locking Doesn't Always Help

Even with clocks locked at 2520 MHz, if the kernel has:
- Poor occupancy (few active warps per SM)
- Memory bottlenecks (waiting on DRAM)
- Serialization (atomic contention)

Then the GPU will still be slow because the cores are idle waiting, not because clocks are low.

## Evidence

1. **nvidia-smi shows 17% SM utilization** - Most cores are idle
2. **Memory utilization is only 4%** - Not memory bandwidth limited
3. **Power draw is only 69W** - GPU is barely working
4. **Performance is 30-400x variable** - Suggests non-deterministic behavior (atomics, power states)

## Possible Root Causes

### Most Likely: Algorithm Implementation Issue

The ICICLE MSM implementation may be:
1. **Not optimized for high-core-count GPUs** - Designed for older GPUs with fewer cores
2. **Using suboptimal bucket size** - Default `c` value doesn't generate enough parallelism
3. **Poor load balancing** - Some SMs get much more work than others
4. **Excessive synchronization** - Too many kernel launches or sync points

### Less Likely: Hardware-Specific Issue

- RTX 4090 architecture (Ada Lovelace) may have different characteristics than what ICICLE was tuned for
- Driver or CUDA version incompatibility

## What This Means

**The fundamental issue is not power management - it's that the ICICLE MSM kernel doesn't efficiently use the RTX 4090's massive parallelism.**

Power management (P0/P5 switching) is a *symptom* of low utilization, not the cause. Even when we force P0 state with clock locking, if the kernel only uses 17% of the GPU, it will still be slow.

## Solutions

### Short Term
1. **Use CPU MSM** - More reliable and faster when GPU is underutilized
2. **Try different problem sizes** - Smaller sizes might have better utilization
3. **Batch multiple MSMs** - May improve GPU utilization

### Long Term
1. **Contact ICICLE developers** - Report RTX 4090 performance issue
2. **Profile with Nsight Compute** - Identify specific bottleneck (occupancy, memory, atomics)
3. **Try different ICICLE versions** - Check if newer/older versions perform better
4. **Consider alternative libraries** - cuZK, Sppark, or other GPU MSM implementations

## Technical Details to Investigate

To truly understand the issue, we need to profile:
1. **Kernel occupancy** - How many warps are active per SM?
2. **Memory throughput** - Are we hitting DRAM bandwidth limits?
3. **Atomic contention** - How much time is spent in atomic operations?
4. **Warp stall reasons** - Why are warps idle? (memory wait, sync, etc.)

This requires running:
```bash
ncu --set full --target-processes all ./msm_binary
```

But this needs proper environment setup for the ICICLE backend to load.

## Conclusion

The 17% GPU utilization is likely due to **insufficient parallelism in the ICICLE MSM kernel for the RTX 4090's 16,384 cores**. The kernel was probably optimized for GPUs with fewer cores (e.g., V100 with 5,120 cores or A100 with 6,912 cores).

Clock locking helps with consistency but doesn't solve the fundamental utilization problem. The CPU implementation is actually more efficient for this workload on RTX 4090.
