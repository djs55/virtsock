package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	ioTimeout = 60 * time.Second
)

var (
	clientStr   string
	serverStr   string
	maxDataLen  int
	connections int
	verbose     int
	exitOnError bool
	connCounter int32
)

// Conn is a net.Conn interface extended with CloseRead/CloseWrite
type Conn interface {
	net.Conn
	CloseRead() error
	CloseWrite() error
}

// Sock is an interface abstracting over the type of socket being used
type Sock interface {
	String() string
	Dial(conid int) (Conn, error)
	Listen() net.Listener
}

// Test is an interface implemented a specific test
type Test interface {
	Server(s Sock)
	Client(s Sock, connid int)
}

func init() {
	flag.StringVar(&clientStr, "c", "", "Start the Client")
	flag.StringVar(&serverStr, "s", "", "Start as a Server")
	flag.IntVar(&maxDataLen, "l", 64*1024, "Maximum Length of data")
	flag.IntVar(&connections, "i", 100, "Total number of connections")
	flag.IntVar(&verbose, "v", 0, "Set the verbosity level")
	exitOnError = true
	flag.Usage = func() {
		prog := filepath.Base(os.Args[0])
		fmt.Printf("USAGE: %s [options]\n\n", prog)
		fmt.Printf("Test socket close behaviour.\n")
		fmt.Printf("A client makes a single connection to the server and starts receiving data.\n")
		fmt.Printf("When the server has sent the data, it will call `Close`. The client counts\n")
		fmt.Printf("the number of bytes received and checks that no in-flight data has been dropped.\n")

		fmt.Printf("-c and -s take a URL as argument (or just the address scheme):\n")
		fmt.Printf("Supported protocols are:\n")
		fmt.Printf("  vsock     virtio sockets (Linux and HyperKit\n")
		fmt.Printf("  hvsock    Hyper-V sockets (Linux and Windows)\n")
		fmt.Printf("  unix      Unix Domain socket\n")
		fmt.Printf("\n")
		fmt.Printf("Note, depending on the Linux kernel version use vsock or hvsock\n")
		fmt.Printf("for Hyper-V sockets (newer kernels use the vsocks interface for Hyper-V sockets.\n")
		fmt.Printf("\n")
		fmt.Printf("Options:\n")
		flag.PrintDefaults()
		fmt.Printf("\n")
		fmt.Printf("Examples:\n")
		fmt.Printf("  %s -s vsock            Start server in vsock mode on standard port\n", prog)
		fmt.Printf("  %s -s vsock://:1235    Start server in vsock mode on a non-standard port\n", prog)
		fmt.Printf("  %s -c hvsock://<vmid>  Start client in hvsock mode connecting to VM with <vmid>\n", prog)
	}
}

func main() {
	log.SetFlags(log.LstdFlags)
	flag.Parse()

	var s Sock
	if serverStr != "" {
		_, s = parseSockStr(serverStr)
	} else {
		_, s = parseSockStr(clientStr)
	}

	t := newStreamCountTest(maxDataLen)

	if serverStr != "" {
		fmt.Printf("Starting server %s\n", s.String())
		t.Server(s)
		return
	}

	fmt.Printf("Client connecting to %s\n", s.String())
	for i := 0; i < connections; i++ {
		err := t.Client(s, i)
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("Test successful.\n")
	return
}

// parseSockStr parses a address of the form <proto>://foo where foo
// is parsed by a proto specific parser
func parseSockStr(inStr string) (string, Sock) {
	u, err := url.Parse(inStr)
	if err != nil {
		log.Fatalf("Failed to parse %s: %v", inStr, err)
	}
	if u.Scheme == "" {
		u.Scheme = inStr
	}
	switch u.Scheme {
	case "vsock":
		return u.Scheme, vsockParseSockStr(u.Host)
	case "hvsock":
		return u.Scheme, hvsockParseSockStr(u.Host)
	case "unix":
		return u.Scheme, unixParseSockStr(u.Path)
	}
	log.Fatalf("Unknown address scheme: '%s'", u.Scheme)
	return "", nil
}
