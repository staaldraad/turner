# Turner

A proof of concept for tunnelling HTTP over a permissive/open TURN server. This will connect to the server, setting up any TCP channels required. A local HTTP proxy is created on 8080, which can be used to "tunnel" the traffic to a target host, for example 169.254.169.254, which the TURN server has access to but you might not have direct access to.

More info: [https://www.rtcsec.com/2020/04/01-slack-webrtc-turn-compromise/](https://www.rtcsec.com/2020/04/01-slack-webrtc-turn-compromise/)

## Install

If using *GO Modules*:

```
git clone https://github.com/staaldraad/turner
cd turner
go build
```

*Otherwise the traditional GOPATH route needs some extra work*

Install the dependencies, this provides the libraries for TURN/STUN that this tool requires:

```
go get gortc.io/turnc # just getting this should be enough, but incase, the other two are below
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

And for the **turn** dependency the same thing;

```
cd $GOPATH/src/gortc.io/turn
git remote add staaldraad https://github.com/staaldraad/turn
git pull staaldraad rfc6062
git checkout rfc6062
```


Finally go this repo:

```
go get github.com/staaldraad/turner
cd $GOPATH/src/github.com/staaldraad/turner
```

## Run

_Disclaimer: Currently this is very much PoC, so things are a bit flaky, YMMV..._

This assumes you already have a TURN server to connect to or are running your own. If you need to run your own checkout: [https://github.com/coturn/coturn/wiki/turnserver](https://github.com/coturn/coturn/wiki/turnserver)

```
./turner -server turn.server:3478
```

You can also supply the username/password if the server requires these:

```
./turner -server turn.server:3478 -u username -p password
```

The proxy listens on **0.0.0.0:8080** by default. 

Testing that the proxy works:

```
# should return your external IP
curl http://ifconf.co/ip 

# should return the IP of the TURN server
curl -x http://localhost:8080 http://ifconf.co/ip 
```


# LICENSE


[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Turner is licensed under a MIT License (https://choosealicense.com/licenses/mit/) 
