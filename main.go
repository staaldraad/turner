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

	"github.com/armon/go-socks5"
	turner "github.com/staaldraad/turner/lib"
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

	stunConnector, err := connectTurn(peer)
	if err != nil {
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

	//ugly hack to recreate same function that could be achieved with httputil.DumpRequest
	// create method line
	methodLine := fmt.Sprintf("%s %s %s\r\n", r.Method, r.URL.Path, r.Proto)
	hostLine := fmt.Sprintf("Host: %s\r\n", target)
	stunConnector.Write([]byte(methodLine))
	stunConnector.Write([]byte(hostLine))
	stunConnector.Write(bufHeader(r.Header))
	stunConnector.Write([]byte("\r\n"))
	//drain body

	io.Copy(stunConnector, r.Body)
	io.Copy(bufwr, stunConnector)

	// close the connections
	defer conn.Close()
	defer stunConnector.Close()
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
	peer := r.Host

	stunConnector, err := connectTurn(peer)
	if err != nil {
		//defer clientConn.Close()
		w.WriteHeader(http.StatusInternalServerError)
		//clientConn.Write([]byte("Proxy encountered error"))
		return
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

	go transfer(stunConnector, clientConn)
	go transfer(clientConn, stunConnector)

}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func connectTurn(target string) (*turner.StunConnection, error) {

	stunConnector := &turner.StunConnection{}

	// Resolving to TURN server.
	raddr, err := net.ResolveTCPAddr("tcp", *server)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	c, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	fmt.Printf("[*] Dial server %s -> %s\n", c.LocalAddr(), c.RemoteAddr())
	client, clientErr := turnc.New(turnc.Options{
		Conn:     c,
		Username: *username,
		Password: *password,
	})
	if clientErr != nil {
		fmt.Println(clientErr)
		c.Close()
		return nil, err
	}
	a, allocErr := client.AllocateTCP()
	if allocErr != nil {
		fmt.Println(allocErr)
		client.Close()
		return nil, err
	}
	peerAddr, resolveErr := net.ResolveTCPAddr("tcp", target)
	if resolveErr != nil {
		client.Close()
		return nil, err
	}
	fmt.Println("[*] Create peer permission")
	permission, createErr := a.Create(peerAddr.IP)
	if createErr != nil {
		client.Close()
		return nil, err
	}
	fmt.Println("[*] Create TCP Session Connection")
	conn, err := permission.CreateTCP(peerAddr)
	if err != nil {
		client.Close()
		return nil, err
	}

	fmt.Println("[*] Create connect request")
	var connid stun.RawAttribute
	if connid, err = conn.Connect(); err != nil {
		client.Close()
		return nil, err
	}

	// setup bind
	fmt.Println("[*] Create bind TCP connection")
	cb, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		client.Close()
		return nil, err
	}

	fmt.Println("[*] Auth and Create client ")
	sideChanReader, sideChanWriter := io.Pipe()
	r := io.MultiReader(sideChanReader, cb)

	clientb, clientErr := turnc.NewData(turnc.Options{
		Conn: cb,
	}, *sideChanWriter)

	if clientErr != nil {
		client.Close()
		return nil, err
	}

	connD, err := permission.CreateTCP(peerAddr)
	if err != nil {
		client.Close()
		return nil, err
	}

	fmt.Println("[*] Bind client ")

	_, err = clientb.ConnectionBind(turn.ConnectionID(binary.BigEndian.Uint32(connid.Value)), a, connD)
	if err != nil {
		client.Close()
		clientb.Close()
		return nil, err
	}
	/*
		buf := make([]byte, 10)
		conn.Read(buf)
		fmt.Println(buf)
	*/
	fmt.Println("[*] Bound")

	stunConnector.CntrClient = *client
	stunConnector.DataClient = *clientb
	stunConnector.Conn = cb
	stunConnector.MultiRead = r

	return stunConnector, nil
}

func turnDial(ctx context.Context, network, addr string) (net.Conn, error) {
	cnn, err := connectTurn(addr)
	if err != nil {
		return nil, err
	}
	return cnn, nil
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
