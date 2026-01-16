#!/bin/bash
export ICICLE_BACKEND_INSTALL_DIR=lib/backend/
export LD_LIBRARY_PATH=lib:lib/backend:/usr/local/cuda/lib64
export CUDA_INJECTION64_PATH=/usr/local/cuda/lib64/libnvperf_host.so

# Try with kernel regex to capture all kernels
ncu --kernel-regex ".*" --launch-count 3 --target-processes all ./msm_binary
