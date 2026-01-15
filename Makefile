# Makefile for MSM Comparison Test

# ICICLE v3 backend configuration
export ICICLE_BACKEND_INSTALL_DIR=/home/cysic/coding/plonky/lighter-prover-fork/lib/backend/
export LD_LIBRARY_PATH=/home/cysic/coding/plonky/lighter-prover-fork/lib:/home/cysic/coding/plonky/lighter-prover-fork/lib/backend:/usr/local/cuda/lib64
export CGO_LDFLAGS=-L/home/cysic/coding/plonky/lighter-prover-fork/lib -L/home/cysic/coding/plonky/lighter-prover-fork/lib/backend -licicle_field_bn254 -licicle_curve_bn254 -licicle_backend_cuda_field_bn254 -licicle_backend_cuda_curve_bn254 -licicle_backend_cuda_device -licicle_device -lstdc++ -Wl,-rpath,/home/cysic/coding/plonky/lighter-prover-fork/lib -Wl,-rpath,/home/cysic/coding/plonky/lighter-prover-fork/lib/backend

.PHONY: help icicle-example icicle-small icicle-medium icicle-large

help:
	@echo "Official ICICLE Example:"
	@echo "  make clean           - Remove binaries"
	@echo "  make icicle-small    - Tiny MSM (2^7 = 128 elements)"
	@echo "  make icicle-medium   - Medium MSM (2^15 = 32K elements)"
	@echo "  make icicle-large    - Large MSM (2^20 = 1M elements)"

icicle-small:
	go run -tags=icicle icicle_official_example.go -size 7 -runs 1

icicle-medium:
	go run -tags=icicle icicle_official_example.go -size 15 -runs 3

icicle-large:
	go run -tags=icicle icicle_official_example.go -size 20 -runs 3

clean:
	go clean -cache