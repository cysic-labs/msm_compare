# Makefile for MSM Comparison Test

# ICICLE v3 backend configuration
export ICICLE_BACKEND_INSTALL_DIR=lib/backend/
export LD_LIBRARY_PATH=lib:lib/backend:/usr/local/cuda/lib64
export CGO_LDFLAGS=-Llib -Llib/backend -licicle_field_bn254 -licicle_curve_bn254 -licicle_backend_cuda_field_bn254 -licicle_backend_cuda_curve_bn254 -licicle_backend_cuda_device -licicle_device -lstdc++ -Wl,-rpath,lib -Wl,-rpath,lib/backend

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

compare:
	go run -tags=icicle main.go -min 18 -max 20 -runs 3

clean:
	go clean -cache

lib:
	mkdir -p lib/backend
	curl -L https://github.com/cysic-labs/msm_compare/releases/download/1.0.0/libicicle_backend_cuda_curve_bn254.so > lib/backend/libicicle_backend_cuda_curve_bn254.so
	curl -L https://github.com/cysic-labs/msm_compare/releases/download/1.0.0/libicicle_backend_cuda_device.so > lib/backend/libicicle_backend_cuda_device.so
	curl -L https://github.com/cysic-labs/msm_compare/releases/download/1.0.0/libicicle_backend_cuda_field_bn254.so > lib/backend/libicicle_backend_cuda_field_bn254.so

	curl -L https://github.com/cysic-labs/msm_compare/releases/download/1.0.0/libicicle_curve_bn254.so > lib/libicicle_curve_bn254.so
	curl -L https://github.com/cysic-labs/msm_compare/releases/download/1.0.0/libicicle_device.so > lib/libicicle_device.so
	curl -L https://github.com/cysic-labs/msm_compare/releases/download/1.0.0/libicicle_field_bn254.so > lib/libicicle_field_bn254.so