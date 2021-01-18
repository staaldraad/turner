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

## Run

This assumes you already have a TURN server to connect to or are running your own. If you need to run your own checkout: [https://github.com/coturn/coturn/wiki/turnserver](https://github.com/coturn/coturn/wiki/turnserver)

```
./turner -server turn.server:3478
```

You can also supply the username/password if the server requires these:

```
./turner -server turn.server:3478 -u username -p password -http
```

The HTTP proxy listens on **127.0.0.1:8080** by default. 

Testing that the proxy works:

```
# should return your external IP
curl http://ifconf.co/ip 

# should return the IP of the TURN server
curl -x http://localhost:8080 http://ifconf.co/ip 
```

### SOCKS5

There is basic SOCKS5 support built-in. The SOCKS5 server can be toggled on via the `-socks5` argument. The proxy listens on port **127.0.0.1:8000** by default.

```
./turner -server turn.server:3478 -u username -p password -socks5
```

It is also possible to enable both SOCKS5 and HTTP proxy-ing at the same time. Simply supply both arguments `-http` and `-socks5`.

# LICENSE


[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Turner is licensed under a MIT License (https://choosealicense.com/licenses/mit/) 
