module github.com/staaldraad/turner

go 1.15

require (
	github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5 // indirect
	golang.org/x/net v0.0.0-20201027133719-8eef5233e2a1 // indirect
	gortc.io/stun v1.22.2
	gortc.io/turn v0.11.2
	gortc.io/turnc v0.2.0
)

replace gortc.io/turn => github.com/staaldraad/turn v0.11.3

replace gortc.io/turnc => github.com/staaldraad/turnc v0.2.7
