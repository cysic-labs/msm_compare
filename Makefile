# Makefile for MSM Comparison Test

# ICICLE v3 backend configuration
V3_LIB=lib
V3_BACKEND=lib/backend
export ICICLE_BACKEND_INSTALL_DIR=$(V3_BACKEND)/

# ICICLE v2 configuration
V2_LIB=/home/cysic/coding/plonky/icicle/examples/golang/msm/lib
V2_INCLUDE=/home/cysic/coding/plonky/icicle/icicle/include

# v3 CGO flags
V3_CGO_LDFLAGS=-L$(V3_LIB) -L$(V3_BACKEND) -licicle_field_bn254 -licicle_curve_bn254 -licicle_backend_cuda_field_bn254 -licicle_backend_cuda_curve_bn254 -licicle_backend_cuda_device -licicle_device -lstdc++ -Wl,-rpath,$(V3_LIB) -Wl,-rpath,$(V3_BACKEND)

# v2 CGO flags
V2_CGO_CFLAGS=-I/usr/local/cuda/include -I$(V2_INCLUDE)
V2_CGO_LDFLAGS=-L$(V2_LIB) -licicle_field_bn254 -licicle_curve_bn254 -lstdc++ -lcudart -L/usr/local/cuda/lib64

.PHONY: help icicle-small icicle-medium icicle-large icicle-v2-small icicle-v2-medium icicle-v2-large compare clean lib

help:
	@echo "ICICLE MSM Benchmark"
	@echo ""
	@echo "Setup:"
	@echo "  make lib             - Download ICICLE v3 libraries (required for v3 tests)"
	@echo "  make clean           - Clear Go build cache"
	@echo ""
	@echo "ICICLE v3 (unstable - has timing issues):"
	@echo "  make icicle-small    - v3 MSM 2^7 = 128 elements"
	@echo "  make icicle-medium   - v3 MSM 2^15 = 32K elements"
	@echo "  make icicle-large    - v3 MSM 2^20 = 1M elements"
	@echo ""
	@echo "ICICLE v2 (stable - recommended):"
	@echo "  make icicle-v2-small  - v2 MSM 2^7 = 128 elements"
	@echo "  make icicle-v2-medium - v2 MSM 2^15 = 32K elements"
	@echo "  make icicle-v2-large  - v2 MSM 2^20 = 1M elements"
	@echo ""
	@echo "Comparison:"
	@echo "  make compare         - CPU vs GPU comparison (v3)"

# ICICLE v3 targets
icicle-small:
	LD_LIBRARY_PATH=$(V3_LIB):$(V3_BACKEND):/usr/local/cuda/lib64 \
	CGO_LDFLAGS="$(V3_CGO_LDFLAGS)" \
	go run -tags=icicle icicle_official_example.go -size 7 -runs 1

icicle-medium:
	LD_LIBRARY_PATH=$(V3_LIB):$(V3_BACKEND):/usr/local/cuda/lib64 \
	CGO_LDFLAGS="$(V3_CGO_LDFLAGS)" \
	go run -tags=icicle icicle_official_example.go -size 15 -runs 3

icicle-large:
	LD_LIBRARY_PATH=$(V3_LIB):$(V3_BACKEND):/usr/local/cuda/lib64 \
	CGO_LDFLAGS="$(V3_CGO_LDFLAGS)" \
	go run -tags=icicle icicle_official_example.go -size 20 -runs 3

# ICICLE v2 targets (stable, recommended)
icicle-v2-small:
	CGO_CFLAGS="$(V2_CGO_CFLAGS)" \
	CGO_LDFLAGS="$(V2_CGO_LDFLAGS)" \
	go run -tags=icicle icicle_official_example_v2.go -size 7 -runs 1

icicle-v2-medium:
	CGO_CFLAGS="$(V2_CGO_CFLAGS)" \
	CGO_LDFLAGS="$(V2_CGO_LDFLAGS)" \
	go run -tags=icicle icicle_official_example_v2.go -size 15 -runs 3

icicle-v2-large:
	CGO_CFLAGS="$(V2_CGO_CFLAGS)" \
	CGO_LDFLAGS="$(V2_CGO_LDFLAGS)" \
	go run -tags=icicle icicle_official_example_v2.go -size 20 -runs 5

compare:
	LD_LIBRARY_PATH=$(V3_LIB):$(V3_BACKEND):/usr/local/cuda/lib64 \
	CGO_LDFLAGS="$(V3_CGO_LDFLAGS)" \
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
