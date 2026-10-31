package main

import (
    "context"
    "crypto/rand"
    "crypto/tls"
    _ "embed"
    "flag"
    "fmt"
    "log"
    "net"
    "net/http"
    "os"
    "os/signal"
    "strconv"
    "syscall"
    "time"
)

//go:embed web/index.html
var indexHTML []byte

// letters/numbers that are hard to mix up when someone reads the link out loud
const keyChars = "abcdefghjkmnpqrstuvwxyz23456789"

func randomKey(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	for i := range b {
		b[i] = keyChars[int(b[i])%len(keyChars)]
	}
	return string(b)
}

// the IPv4 addresses of this machine (the ones other people on the LAN can reach)
func lanIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	return out
}

func main() {
	port := flag.Int("port", 8443 "port to listen on")
	room := flag.String("room", "strikechat", "room name shown at the top")
	key := flag.String("key", "", "key in the join link (random if empty)")
	noKey := flag.Bool("no-key", false, "anyone on the network can join, no key")
	keep := flag.Int("keep", 500, "how many messages to keep in memory")
	maxClients := flag.Int("max-clients", 200, "max people connected at once")
	certFile := flag.String("cert", "cert.pem", "TLS certificate file")
    tlsKeyFile := flag.String("tls-key", "key.pem", "TLS private key file")
	flag.Parse()
	cert, err := tls.LoadX509KeyPair(*certFile, *tlsKeyFile)
      if err != nil {
    log.Fatalf("failed to load TLS certificate: %v", err)
    }

	k := *key
	if *noKey {
		k = ""
	} else if k == "" {
		k = randomKey(6)
	}

	hub := NewHub(*keep)
	srv := NewServer(*room, k, hub, indexHTML, *maxClients)

	httpSrv := &http.Server{
		Addr:              ":" + strconv.Itoa(*port),
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// no WriteTimeout on purpose, the live stream stays open for a long time
		TLSConfig: &tls.Config{
        MinVersion:   tls.VersionTLS12,
        Certificates: []tls.Certificate{cert},
    },
	}
	httpSrv.RegisterOnShutdown(func() { close(srv.done) })

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// clean old rate limit entries every few minutes
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				srv.sendLim.Cleanup(10 * time.Minute)
				srv.authLim.Cleanup(10 * time.Minute)
			}
		}
	}()

	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		httpSrv.Shutdown(sctx)
	}()

	fmt.Printf("\n  strikechat  room: %q\n\n  send this link to people on the same network:\n\n", *room)
	ips := lanIPs()
	if len(ips) == 0 {
		fmt.Println("  (couldn't find a network address, are you connected?)")
	}
	for _, ip := range ips {
		if k == "" {
			fmt.Printf("    http://%s:%d/\n", ip, *port)
		} else {
			fmt.Printf("    http://%s:%d/?k=%s\n", ip, *port, k)
		}
	}
	fmt.Print("\n  (if you have a few addresses, use the one that matches the college network)\n")
	fmt.Print("  messages are only kept in memory. Ctrl+C stops the server and wipes everything.\n\n")

	if err := httpSrv.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
    log.Fatal(err)
}
