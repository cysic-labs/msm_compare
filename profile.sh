export ICICLE_BACKEND_INSTALL_DIR=lib/backend/
export LD_LIBRARY_PATH=lib:lib/backend:/usr/local/cuda/lib64
ncu --set full --target-processes all ./msm_binary