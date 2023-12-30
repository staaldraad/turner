module github.com/staaldraad/turner

go 1.20

require (
	github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5
	gortc.io/stun v1.22.2
	gortc.io/turn v0.11.2
	gortc.io/turnc v0.2.0
)

require (
	go.uber.org/atomic v1.4.0 // indirect
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.10.0 // indirect
	golang.org/x/net v0.17.0 // indirect
)

replace gortc.io/turn => github.com/staaldraad/turn v0.11.3

replace gortc.io/turnc => github.com/staaldraad/turnc v0.3.1

//replace gortc.io/turnc => ../turnc

replace gortc.io/stun => github.com/staaldraad/stun v1.22.3
