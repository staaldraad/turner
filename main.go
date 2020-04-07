package main

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"gortc.io/stun"
	"gortc.io/turn"
	"gortc.io/turnc"
)

var (
	server = flag.String("server",
		fmt.Sprintf("localhost:3478"),
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
	fmt.Println(r.Method)

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
	fmt.Println(peer)

	dest_conn, err := connectTurn(peer)
	if err != nil {

	}

	// create method line
	methodLine := fmt.Sprintf("%s %s %s\r\n", r.Method, r.URL.Path, r.Proto)
	hostLine := fmt.Sprintf("Host: %s\r\n", target)
	dest_conn.Write([]byte(methodLine))
	dest_conn.Write([]byte(hostLine))
	dest_conn.Write(bufHeader(r.Header))
	dest_conn.Write([]byte("\r\n"))
	//drain body

	io.Copy(dest_conn, r.Body)

	defer dest_conn.Close()

	dest_conn.SetReadBuffer(2048)

	timeoutDuration := 5 * time.Second
	bufReader := bufio.NewReader(dest_conn)

	for {
		// Set a deadline for reading. Read operation will fail if no data
		// is received after deadline.
		dest_conn.SetReadDeadline(time.Now().Add(timeoutDuration))

		// Read tokens delimited by newline
		bytes, err := bufReader.ReadBytes('\n')
		if err != nil {
			fmt.Println(err)
			break
		}

		fmt.Printf("%s", bytes)
		w.Write(bytes)
	}
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
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
	client_conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}

	dest_conn, err := connectTurn(peer)
	if err != nil {

	}
	go transfer(dest_conn, client_conn)
	go transfer(client_conn, dest_conn)

}

func connectTurn(target string) (*net.TCPConn, error) {
	// Resolving to TURN server.
	raddr, err := net.ResolveTCPAddr("tcp", *server)
	if err != nil {
		panic(err)
	}
	c, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("dial server %s -> %s\n", c.LocalAddr(), c.RemoteAddr())
	client, clientErr := turnc.New(turnc.Options{
		Conn:     c,
		Username: *username,
		Password: *password,
	})
	if clientErr != nil {
		panic(clientErr)
	}
	a, allocErr := client.AllocateTCP()
	if allocErr != nil {
		panic(allocErr)
	}
	peerAddr, resolveErr := net.ResolveTCPAddr("tcp", target)
	if resolveErr != nil {
		panic(resolveErr)
	}
	fmt.Println("create peer")
	permission, createErr := a.Create(peerAddr.IP)
	if createErr != nil {
		panic(createErr)
	}
	fmt.Println("create peer")
	conn, err := permission.CreateTCP(peerAddr)
	if err != nil {
		panic(err)
	}

	fmt.Println("send connect request")
	var connid stun.RawAttribute
	if connid, err = conn.Connect(); err != nil {
		fmt.Println(err)
		return nil, err
	}

	// setup bind
	fmt.Println("setting up bind")
	cb, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		panic(err)
	}
	clientb, clientErr := turnc.New(turnc.Options{
		Conn:     cb,
		Username: *username,
		Password: *password,
	})
	if clientErr != nil {
		panic(clientErr)
	}

	err = clientb.ConnectionBind(turn.ConnectionID(binary.BigEndian.Uint32(connid.Value)))
	if err != nil {
		panic(err)
	}
	return cb, err
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
