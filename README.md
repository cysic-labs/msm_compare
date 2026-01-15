# Test Icicle MSM time

The ICICLE GPU MSM time is not stable as for $2^20$ elements, the time some time is about 6~7 seconds and sometime is less than 20 milliseconds.

![msm_time](./pic/msm_time.png)

# How to run

```bash
make lib # download the icicle bn254 lib and backend lib
make icicle-large # run the icicle large MSM test and get msm time
```