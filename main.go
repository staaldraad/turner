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

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("This is a HTTP Proxy, use it as such"))
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func handleProxyTun(w http.ResponseWriter, r *http.Request) {
	target := ""

	// if GET http://address parse out the address
	if r.Method == http.MethodGet {
		fmt.Println(r.URL.Host)
		target = r.URL.Host
	}
	target = r.URL.Host
	if target == "" {
		w.Write([]byte("This is a HTTP Proxy, use it as such"))
		return
	}

	port := r.URL.Port()

	if port == "" {
		port = "80"
	}
	//peer := fmt.Sprintf("%s:%s", target, port)
	peer := target
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
	peerAddr, resolveErr := net.ResolveTCPAddr("tcp", peer)
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
		panic(err)
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

	//dst_r := bufio.NewReader(cb)
	//dst_w := bufio.NewWriter(cb)
	fmt.Fprint(cb, []byte("baaaa"))
	fmt.Fprint(client_conn, cb.CloseRead().Error())
	//fmt.Println(client_conn)

}

func main() {
	flag.Parse()

	server := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect || r.Method == http.MethodGet {
				//handleTunneling(w, r)
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
