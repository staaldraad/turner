module github.com/staaldraad/turner

go 1.15

require (
	github.com/acomagu/bufpipe v1.0.3 // indirect
	github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5
	golang.org/x/net v0.0.0-20201029055024-942e2f445f3c // indirect
	gortc.io/stun v1.22.2
	gortc.io/turn v0.11.2
	gortc.io/turnc v0.2.0
)

replace gortc.io/turn => github.com/staaldraad/turn v0.11.3

replace gortc.io/turnc => github.com/staaldraad/turnc v0.2.10
//replace gortc.io/turnc => ../turnc

replace gortc.io/stun => github.com/staaldraad/stun v1.22.3
