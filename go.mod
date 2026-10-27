module msm_compare

go 1.24.9

// replace github.com/consensys/gnark => ../external/gnark

replace github.com/ingonyama-zk/icicle/v2 => /home/cysic/coding/plonky/icicle

require (
	github.com/consensys/gnark-crypto v0.18.1
	github.com/ingonyama-zk/icicle-gnark/v3 v3.2.2
	github.com/ingonyama-zk/icicle/v2 v2.0.0
	github.com/ingonyama-zk/icicle/v3 v3.9.2
)

require (
	github.com/bits-and-blooms/bitset v1.20.0 // indirect
	github.com/consensys/bavard v0.1.31-0.20250406004941-2db259e4b582 // indirect
	github.com/mmcloughlin/addchain v0.4.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/sys v0.30.0 // indirect
	rsc.io/tmplfunc v0.0.3 // indirect
)
