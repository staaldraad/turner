package main

import (
	"crypto/tls"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"gortc.io/stun"
	"gortc.io/turn"
	"gortc.io/turnc"
)

var (
	server = flag.String("server",
		"localhost:3478",
		"turn server address",
	)

	username = flag.String("u", "user", "username")
	password = flag.String("p", "secret", "password")
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

	controlConn, destConn, err := connectTurn(peer)
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

	//destConn.SetReadBuffer(512)

	timeoutDuration := 5 * time.Second
	//bufReader := bufio.NewReader(destConn)
	//buf := make([]byte, 2048)

	for {
		// Set a deadline for reading. Read operation will fail if no data
		// is received after deadline.
		destConn.SetReadDeadline(time.Now().Add(timeoutDuration))

		_, err := io.Copy(bufwr, destConn)
		if err != nil {
			if err == io.EOF {
				continue
			}
			break
		}
		/*
			// Read tokens delimited by newline
			bytes, err := destConn.Read()
			if err != nil {
				fmt.Println(err)
				break
			}
		*/
		//fmt.Printf("%s", bytes)
		//bufwr.Write(buf[:n])
		//bufwr.Flush()
	}
	// close the connections
	closer.Do(closeFunc)

}

func transfer(destination io.WriteCloser, source io.ReadCloser, closer *sync.Once) {
	closeFunc := func() {
		fmt.Println("[*] Connections closed.")
		_ = destination.Close()
		_ = source.Close()
	}
	io.Copy(destination, source)
	closer.Do(closeFunc)
}

func handleProxyTun(w http.ResponseWriter, r *http.Request) {
	fmt.Println("CONNECT")

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

	controlConn, destConn, err := connectTurn(peer)
	if err != nil {
		if controlConn != nil {
			controlConn.Close()
		}
		if destConn != nil {
			destConn.Close()
		}
		clientConn.Write([]byte("Proxy encountered error"))
	}

	// make sure connections get closed, use a sync.Once to ensure the close happens in one of the handlers
	var closer sync.Once

	go transfer(destConn, clientConn, &closer)
	transfer(clientConn, destConn, &closer)

}

func connectTurn(target string) (*net.TCPConn, *net.TCPConn, error) {
	// Resolving to TURN server.
	raddr, err := net.ResolveTCPAddr("tcp", *server)
	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}
	c, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		fmt.Println(err)
		return nil, nil, err
	}
	fmt.Printf("[*] Dial server %s -> %s\n", c.LocalAddr(), c.RemoteAddr())
	client, clientErr := turnc.New(turnc.Options{
		Conn:     c,
		Username: *username,
		Password: *password,
	})
	if clientErr != nil {
		fmt.Println(clientErr)
		return c, nil, err
	}
	//client.StartRead()
	a, allocErr := client.AllocateTCP()
	if allocErr != nil {
		fmt.Println(allocErr)
		return c, nil, err
	}
	peerAddr, resolveErr := net.ResolveTCPAddr("tcp", target)
	if resolveErr != nil {
		fmt.Println(resolveErr)
		return c, nil, err
	}
	fmt.Println("[*] Create peer permission")
	permission, createErr := a.Create(peerAddr.IP)
	if createErr != nil {
		fmt.Println(createErr)
		return c, nil, err
	}
	fmt.Println("[*] Create TCP Session Connection")
	conn, err := permission.CreateTCP(peerAddr)
	if err != nil {
		fmt.Println(err)
		return c, nil, err
	}

	fmt.Println("[*] Create connect request")
	var connid stun.RawAttribute
	if connid, err = conn.Connect(); err != nil {
		fmt.Println(err)
		return c, nil, err
	}

	// setup bind
	fmt.Println("[*] Create bind TCP connection")
	cb, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		fmt.Println(err)
		return c, nil, err
	}

	fmt.Println("[*] Auth and Create client ")
	clientb, clientErr := turnc.New(turnc.Options{
		Conn:     cb,
		Username: *username,
		Password: *password,
	})
	if clientErr != nil {
		fmt.Println(clientErr)
		return c, cb, err
	}

	fmt.Println("[*] Bind client ")
	err = clientb.ConnectionBind(turn.ConnectionID(binary.BigEndian.Uint32(connid.Value)), a)
	if err != nil {
		fmt.Println("[x] Couldn't bind", err)
		return c, cb, err
	}
	//clientb.StartRead()
	fmt.Println("[*] Bound")
	return c, cb, nil
}

func main() {
	flag.Parse()

	server := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				handleProxyTun(w, r)
			} else {
				handleHTTP(w, r)
			}
		}),
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	log.Fatal(server.ListenAndServe())

}
