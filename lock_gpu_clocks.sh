#!/bin/bash
# GPU Clock Locking Script for Stable MSM Performance
#
# This script locks the GPU clocks to prevent the GPU from entering low-power
# states (P5) which cause 100-400x performance degradation in ICICLE MSM.
#
# MUST BE RUN WITH SUDO

set -e

echo "========================================="
echo "GPU Clock Locking for ICICLE MSM"
echo "========================================="
echo

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This script must be run with sudo"
    echo "Usage: sudo ./lock_gpu_clocks.sh"
    exit 1
fi

# Get GPU info
echo "Current GPU status:"
nvidia-smi --query-gpu=index,name,pstate,clocks.current.graphics,clocks.max.graphics --format=csv
echo

# Lock GPU 0 clocks to boost range (2520-3105 MHz)
echo "Locking GPU 0 clocks to 2520-3105 MHz..."
nvidia-smi -lgc 2520,3105 -i 0

# Lock memory clocks to max (10501 MHz)
echo "Locking GPU 0 memory clocks to 10501 MHz..."
nvidia-smi -lmc 10501 -i 0

echo
echo "GPU clocks locked successfully!"
echo
echo "New GPU status:"
nvidia-smi --query-gpu=index,name,pstate,clocks.current.graphics,clocks.current.memory --format=csv
echo

echo "========================================="
echo "To reset clocks to default (unlock):"
echo "  sudo nvidia-smi -rgc -i 0"
echo "  sudo nvidia-smi -rmc -i 0"
echo "========================================="
