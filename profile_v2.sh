#!/bin/bash
export ICICLE_BACKEND_INSTALL_DIR=lib/backend/
export LD_LIBRARY_PATH=lib:lib/backend:/usr/local/cuda/lib64

# Profile with basic metrics, all processes, capture first 5 kernels
ncu --metrics sm__throughput.avg.pct_of_peak_sustained_elapsed,dram__throughput.avg.pct_of_peak_sustained_elapsed,gpu__compute_memory_throughput.avg.pct_of_peak_sustained_elapsed --launch-count 5 --target-processes all ./msm_binary
