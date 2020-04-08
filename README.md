## Install

Install the dependencies, this provides the libary for TURN/STUN that this tool requires:

```
go get gortc.io/turnc
go get gortc.io/turn
go get gortc.io/stun
```

Pull in the changes that enable TCP binding based on RFC6062

```
cd $GOPATH/src/gortc.io/turnc
git remote add staaldraad https://github.com/staaldraad/turnc
git pull staaldraad rfc6062
git checkout rfc6062
```

And for the turn dependency the same thing;

```
cd $GOPATH/src/gortc.io/turn
git remote add staaldraad https://github.com/staaldraad/turn
git pull staaldraad rfc6062
git checkout rfc6062
```


Finally go this repo:

```
go get github.com/staaldraad/pubstun
cd $GOPATH/src/github.com/staaldraad/pubstun
```

## Run

_Disclaimer: Currently this is very much PoC, so things are a bit flaky, YMMV..._

This assumes you already have a TURN server to connect to or are running your own. If you need to run your own checkout: [https://github.com/coturn/coturn/wiki/turnserver](https://github.com/coturn/coturn/wiki/turnserver)

```
go run main.go -server turn.server:3478
```

You can also supply the username/password if the server requires these:

```
go run main.go -server turn.server:3478 -u username -p password
```

The proxy listens on **0.0.0.0:8080** by default. 

