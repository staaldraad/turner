module github.com/staaldraad/turner

require (
	gortc.io/stun v1.22.1
	gortc.io/turn v0.11.2
	gortc.io/turnc v0.2.0
)

replace gortc.io/turn => ../turn

replace gortc.io/turnc => ../turnc
