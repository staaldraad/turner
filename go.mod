module github.com/staaldraad/turner

go 1.15

require (
	gortc.io/stun v1.22.1
	gortc.io/turn v0.11.2
	gortc.io/turnc v0.2.0
)

replace gortc.io/turn => github.com/staaldraad/turn v0.11.3

replace gortc.io/turnc => github.com/staaldraad/turnc v0.2.2
