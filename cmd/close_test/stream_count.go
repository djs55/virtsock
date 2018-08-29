package main

// This streams a fixed amount of data from the server to the client
// and calls Close. The client verifies that all data was received and
// none was lost in transit.

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"time"
)

type streamCount struct {
	dataLen int
}

func newStreamCountTest(dataLen int) streamCount {
	return streamCount{
		dataLen: dataLen,
	}
}

func (t streamCount) Server(s Sock) {
	l := s.Listen()
	defer l.Close()

	connid := 0

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatalf("Accept(): %s\n", err)
		}

		prDebug("[%05d] accept(): %s -> %s \n", connid, conn.RemoteAddr(), conn.LocalAddr())
		go t.handleRequest(conn, connid)
		connid++
	}
}

func (t streamCount) handleRequest(c net.Conn, connid int) {
	defer func() {
		prDebug("[%05d] Closing\n", connid)
		err := c.Close()
		if err != nil {
			prError("[%05d] Close(): %s\n", connid, err)
		}
	}()

	txbuf := randBuf(t.dataLen)

	start := time.Now()

	l, err := c.Write(txbuf)
	if err != nil {
		prError("[%05d]: Write failed with %v", err)
		return
	}
	if l != t.dataLen {
		prError("[%05d]: Write returned short: %d < %d", l, t.dataLen)
		return
	}

	diffTime := time.Since(start)
	prInfo("[%05d] WRITTEN: %10d bytes in %10.4f ms\n", connid, t.dataLen, diffTime.Seconds()*1000)
}

func (t streamCount) Client(s Sock, conid int) error {
	c, err := s.Dial(conid)
	if err != nil {
		prError("[%05d] Failed to Dial: %s %s\n", conid, s, err)
		return err
	}
	defer c.Close()

	n, err := io.Copy(ioutil.Discard, c)
	if err != nil {
		prError("[%05d] io.Copy failed with %v", conid, err)
		return err
	}
	prDebug("[%05d] Received %d bytes", conid, n)
	if int(n) != t.dataLen {
		prError("[%05d]: Read returned too few bytes: %d < %Ld", n, t.dataLen)
		return fmt.Errorf("Close dropped in-flight data")
	}
	return nil
}
