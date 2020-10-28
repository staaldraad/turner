package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/armon/go-socks5"
	"gortc.io/stun"
	"gortc.io/turn"
	"gortc.io/turnc"
)

var (
	server = flag.String("server",
		"localhost:3478",
		"turn server address",
	)

	username  = flag.String("u", "user", "username")
	password  = flag.String("p", "secret", "password")
	socksProx = flag.Bool("socks5", false, "Start a SOCKS5 server")
	httpProx  = flag.Bool("http", false, "Start HTTP Proxy")
	socksPort = flag.Int("sp", 8000, "Port to use for SOCKS server")
	httpPort  = flag.Int("hp", 8080, "Port to use for HTTP Proxy")
	socksHost = flag.String("sh", "127.0.0.1", "Host addr to listen on SOCKS5 (default 127.0.0.1)")
	httpHost  = flag.String("hh", "127.0.0.1", "Host addr to listen on HTTP (default 127.0.0.1)")
)

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func bufHeader(src http.Header) []byte {
	buf := make([]byte, 0)
	for k, vv := range src {
		buf = append(buf, []byte(k)...)
		buf = append(buf, []byte(":")...)
		for _, v := range vv {
			buf = append(buf, []byte(v)...)
		}
		buf = append(buf, []byte("\r\n")...)
	}
	return buf
}

// this function is such an ugly hack but I'm tired and it works
// look at replacing with real code that does io.Copy and
// better buffer handling
// this drains http headers, constructs manual method line
// and manual host line
// then sends everything to the server
func handleHTTP(w http.ResponseWriter, r *http.Request) {

	target := r.URL.Host
	if target == "" {
		w.Write([]byte("This is a HTTP Proxy, use it as such"))
		return
	}

	port := r.URL.Port()

	if port == "" {
		port = "80"
	}
	peer := target
	if strings.Index(target, ":") == -1 {
		peer = fmt.Sprintf("%s:%s", target, port)
	}
	fmt.Printf("[*] Proxy to peer: %s\n", peer)

	var closer sync.Once

	controlConn, destConn, client, clientb, err := connectTurn(peer)
	if err != nil {
		if controlConn != nil {
			controlConn.Close()
		}
		if destConn != nil {
			destConn.Close()
		}
		http.Error(w, "Proxy encountered error", http.StatusInternalServerError)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	conn, bufwr, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Don't forget to close the connection:
	//defer conn.Close()

	// make sure connections get closed
	closeFunc := func() {
		fmt.Println("[*] Connections closed")
		_ = controlConn.Close()
		_ = destConn.Close()
		_ = conn.Close()
		_ = client.Close()
		_ = clientb.Close()
	}

	//ugly hack to recreate same function that could be achieved with httputil.DumpRequest
	// create method line
	methodLine := fmt.Sprintf("%s %s %s\r\n", r.Method, r.URL.Path, r.Proto)
	hostLine := fmt.Sprintf("Host: %s\r\n", target)
	destConn.Write([]byte(methodLine))
	destConn.Write([]byte(hostLine))
	destConn.Write(bufHeader(r.Header))
	destConn.Write([]byte("\r\n"))
	//drain body

	io.Copy(destConn, r.Body)

	/*
		// Would have loved to just use DumpRequest here
		// but this drops the Host header as go follows rfc7230
		// https://github.com/golang/go/issues/16265
		// which ends up giving problems as receiving servers return 400 "missing host header"
		dump, err := httputil.DumpRequest(r, true)
		if err != nil {
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
			return
		}
		destConn.Write(dump)
		fmt.Printf("%q\n", dump)
	*/

	destConn.SetReadBuffer(512)

	timeoutDuration := 5 * time.Second
	// Set a deadline for reading. Read operation will fail if no data
	// is received after deadline.
	destConn.SetReadDeadline(time.Now().Add(timeoutDuration))

	for {

		_, err := io.Copy(bufwr, destConn)
		if err != nil {
			break
		}
	}
	// close the connections
	closer.Do(closeFunc)

}

func transfer(destination io.WriteCloser, source io.ReadCloser, c, cb *turnc.Client, closer *sync.Once) {
	closeFunc := func() {
		fmt.Println("[*] Connections closed.")
		_ = destination.Close()
		_ = source.Close()
		_ = c.Close()
		_ = cb.Close()
	}
	io.Copy(destination, source)
	closer.Do(closeFunc)
}

func handleProxyTun(w http.ResponseWriter, r *http.Request) {

	target := r.URL.Host
	if target == "" {
		w.Write([]byte("This is a HTTP Proxy, use it as such"))
		return
	}

	port := r.URL.Port()

	if port == "" {
		port = "80"
	}
	peer := target
	if strings.Index(target, ":") == -1 {
		peer = fmt.Sprintf("%s:%s", target, port)
	}

	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}

	controlConn, destConn, client, clientb, err := connectTurn(peer)
	if err != nil {
		if controlConn != nil {
			controlConn.Close()
		}
		if destConn != nil {
			destConn.Close()
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
		//clientConn.Write([]byte("Proxy encountered error"))
	}

	// make sure connections get closed, use a sync.Once to ensure the close happens in one of the handlers
	var closer sync.Once

	go transfer(destConn, clientConn, client, clientb, &closer)
	transfer(clientConn, destConn, client, clientb, &closer)

	clientConn.Close()
}

func connectTurn(target string) (*net.TCPConn, *net.TCPConn, *turnc.Client, *turnc.Client, error) {
	// Resolving to TURN server.
	raddr, err := net.ResolveTCPAddr("tcp", *server)
	if err != nil {
		fmt.Println(err)
		return nil, nil, nil, nil, err
	}
	c, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		fmt.Println(err)
		return nil, nil, nil, nil, err
	}
	fmt.Printf("[*] Dial server %s -> %s\n", c.LocalAddr(), c.RemoteAddr())
	client, clientErr := turnc.New(turnc.Options{
		Conn:     c,
		Username: *username,
		Password: *password,
	})
	if clientErr != nil {
		fmt.Println(clientErr)
		return c, nil, nil, nil, err
	}
	a, allocErr := client.AllocateTCP()
	if allocErr != nil {
		fmt.Println(allocErr)
		return c, nil, nil, nil, err
	}
	peerAddr, resolveErr := net.ResolveTCPAddr("tcp", target)
	if resolveErr != nil {
		fmt.Println(resolveErr)
		return c, nil, nil, nil, err
	}
	fmt.Println("[*] Create peer permission")
	permission, createErr := a.Create(peerAddr.IP)
	if createErr != nil {
		fmt.Println(createErr)
		return c, nil, nil, nil, err
	}
	fmt.Println("[*] Create TCP Session Connection")
	conn, err := permission.CreateTCP(peerAddr)
	if err != nil {
		fmt.Println(err)
		return c, nil, nil, nil, err
	}

	fmt.Println("[*] Create connect request")
	var connid stun.RawAttribute
	if connid, err = conn.Connect(); err != nil {
		fmt.Println(err)
		return c, nil, nil, nil, err
	}

	// setup bind
	fmt.Println("[*] Create bind TCP connection")
	cb, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		fmt.Println(err)
		return c, nil, nil, nil, err
	}

	fmt.Println("[*] Auth and Create client ")
	clientb, clientErr := turnc.New(turnc.Options{
		Conn: cb,
	})
	if clientErr != nil {
		fmt.Println(clientErr)
		return c, nil, client, clientb, err
	}

	time.Sleep(500 * time.Millisecond)

	fmt.Println("[*] Bind client ")
	_, err = clientb.ConnectionBind(turn.ConnectionID(binary.BigEndian.Uint32(connid.Value)), a)
	if err != nil {
		fmt.Println("[x] Couldn't bind", err)
		return c, cb, client, clientb, err
	}

	fmt.Println("[*] Bound")
	return c, cb, client, clientb, nil
}

func turnDial(ctx context.Context, network, addr string) (net.Conn, error) {
	ctlCon, dataCon, ctlClient, dataClient, err := connectTurn(addr)
	if err != nil {
		return nil, err
	}

	// quick hack function to close connections
	go func() {
		b := make([]byte, 0)
		for {
			if _, e := dataCon.Read(b); e != nil {
				ctlCon.Close()
				ctlClient.Close()
				dataCon.Close()
				dataClient.Close()
				break
			}
		}
	}()
	return dataCon, nil
}

func main() {
	flag.Parse()

	if !*httpProx && !*socksProx {
		fmt.Println("[x] No mode selected. Use either, or both, -http or -socks5")
		return
	}
	errChan := make(chan error)

	if *httpProx {
		go func(errChan chan error) {
			fmt.Printf("[*] Starting HTTP Server on %s:%d\n", *httpHost, *httpPort)
			httpServer := &http.Server{
				Addr: fmt.Sprintf("%s:%d", *httpHost, *httpPort),
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodConnect {
						handleProxyTun(w, r)
					} else {
						handleHTTP(w, r)
					}
				}),
				// Disable HTTP/2.
				//TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
			}
			errChan <- httpServer.ListenAndServe()
		}(errChan)
	}

	if *socksProx {
		fmt.Printf("[*] Starting SOCKS5 Server on %s:%d\n", *socksHost, *socksPort)
		go func(errChan chan error) {
			conf := &socks5.Config{Dial: turnDial}
			server, err := socks5.New(conf)
			if err != nil {
				errChan <- err
				return
			}

			// Create SOCKS5 proxy on localhost port 8000
			errChan <- server.ListenAndServe("tcp", fmt.Sprintf("%s:%d", *socksHost, *socksPort))
		}(errChan)
	}

	select {
	case <-errChan:
		fmt.Println("Error setting up server.", errChan)

	}
}
