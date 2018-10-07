package shutdown

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"

	"encoding/binary"
)

// Wrap hypervisor socket connections with a simple "shutdown" protocol to
// work around bugs in the socket implementation.

// From github.com/linuxkit/virtsock @ db053ed33b0c5cda489f4fb2d5583d98cbf55547
//
// There is an additional wrinkle. Hyper-V sockets in currently
// shipping versions of Windows don't support graceful and/or
// unidirectional shutdown(). So we turn a stream based protocol into
// message based protocol which allows to send in-line "messages" to
// the other end. We then provide a stream based interface on top of
// that. Yuk.
//
// The message interface is pretty simple. We first send a 32bit
// message containing the size of the data in the following
// message. Messages are limited to 'maxmsgsize'. Special message
// (without data), `shutdownrd` and 'shutdownwr' are used to used to
// signal a shutdown to the other end.

// On Windows 10 build 10586 larger maxMsgSize values work, but on
// newer builds it fails. It is unclear what the cause is...
const (
	maxMsgSize = 4 * 1024 // Maximum message size
)

var (
	// Debug enables additional debug output
	Debug = false
)

const (
	shutdownrd = 0xdeadbeef // Message for CloseRead()
	shutdownwr = 0xbeefdead // Message for CloseWrite()
	closemsg   = 0xdeaddead // Message for Close()
)

// Conn is a connection which supports half-close.
type Conn interface {
	net.Conn
	CloseRead() error
	CloseWrite() error
}

/*
 * A wrapper around FileConn which supports CloseRead and CloseWrite
 */

var (
	// ErrSocketClosed is returned when an operation is attempted on a socket which has been closed
	ErrSocketClosed = errors.New("HvSocket has already been closed")
	// ErrSocketWriteClosed is returned on a write when the socket has been closed for write
	ErrSocketWriteClosed = errors.New("HvSocket has been closed for write")
	// ErrSocketReadClosed is returned on a write when the socket has been closed for read
	ErrSocketReadClosed = errors.New("HvSocket has been closed for read")
	// ErrSocketMsgSize is returned a message has the wrong size
	ErrSocketMsgSize = errors.New("HvSocket message was of wrong size")
	// ErrSocketMsgWrite is returned when a message write failed
	ErrSocketMsgWrite = errors.New("HvSocket writing message")
	// ErrSocketNotEnoughData is returned when not all data could be written
	ErrSocketNotEnoughData = errors.New("HvSocket not enough data written")
	// ErrSocketUnImplemented is returned a function is not implemented
	ErrSocketUnImplemented = errors.New("Function not implemented")
)

// WrappedConn maintains the state of a hypervisor socket connection
type wrappedConn struct {
	net.Conn

	wrlock *sync.Mutex

	writeClosed bool
	readClosed  bool

	bytesToRead int
}

// Open returns a Conn which supports half-close
func Open(conn net.Conn) Conn {
	var wrlock sync.Mutex
	return &wrappedConn{
		Conn:   conn,
		wrlock: &wrlock,
	}
}

// LocalAddr returns the local address of the hypervisor socket connection
func (w *wrappedConn) LocalAddr() net.Addr {
	return w.Conn.LocalAddr()
}

// RemoteAddr returns the remote address of the hypervisor socket connection
func (w *wrappedConn) RemoteAddr() net.Addr {
	return w.Conn.RemoteAddr()
}

// Close closes a connection
func (w *wrappedConn) Close() error {
	prDebug("Close\n")

	w.readClosed = true
	w.writeClosed = true

	prDebug("TX: Close\n")
	w.wrlock.Lock()
	err := w.sendMsg(closemsg)
	w.wrlock.Unlock()
	if err != nil {
		// chances are that the other end beat us to the close
		prDebug("Mmmm. %s\n", err)
		return w.Conn.Close()
	}

	// wait for reply/ignore errors
	// we may get a EOF because the other end  closed,
	b := make([]byte, 4)
	_, _ = w.Conn.Read(b)
	prDebug("close\n")
	return w.Conn.Close()
}

// CloseRead closes a connection for reading
func (w *wrappedConn) CloseRead() error {
	if w.readClosed {
		return ErrSocketReadClosed
	}

	prDebug("TX: Shutdown Read\n")
	w.wrlock.Lock()
	err := w.sendMsg(shutdownrd)
	w.wrlock.Unlock()
	if err != nil {
		return err
	}

	w.readClosed = true
	return nil
}

// CloseWrite closes a connection for writing
func (w *wrappedConn) CloseWrite() error {
	if w.writeClosed {
		return ErrSocketWriteClosed
	}

	prDebug("TX: Shutdown Write\n")
	w.wrlock.Lock()
	err := w.sendMsg(shutdownwr)
	w.wrlock.Unlock()
	if err != nil {
		return err
	}

	w.writeClosed = true
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Read into buffer
// Also handles the inband control messages.
func (w *wrappedConn) Read(buf []byte) (int, error) {
	if w.readClosed {
		return 0, io.EOF
	}

	if w.bytesToRead == 0 {
		for {
			// wait for next message
			b := make([]byte, 4)

			n, err := w.Conn.Read(b)
			if err != nil {
				return 0, err
			}

			if n != 4 {
				return n, ErrSocketMsgSize
			}

			msg := int(binary.LittleEndian.Uint32(b))
			if msg == shutdownwr {
				// The other end shutdown write. No point reading more
				w.readClosed = true
				prDebug("RX: ShutdownWrite\n")
				return 0, io.EOF
			} else if msg == shutdownrd {
				// The other end shutdown read. No point writing more
				w.writeClosed = true
				prDebug("RX: ShutdownRead\n")
			} else if msg == closemsg {
				// Setting write close here forces a proper close
				w.writeClosed = true
				prDebug("RX: Close\n")
				w.Conn.Close()
			} else {
				w.bytesToRead = msg
				if w.bytesToRead == 0 {
					// XXX Something is odd. If I don't have this here, this
					// case is hit. However, with this code in place this
					// case never get's hit. Suspect overly eager GC...
					log.Printf("RX: Zero length %02x", b)
					continue
				}
				break
			}
		}
	}

	// If we get here, we know there is v.bytesToRead worth of
	// data coming our way. Read it directly into to buffer passed
	// in by the caller making sure we do not read mode than we
	// should read by splicing the buffer.
	toRead := min(len(buf), w.bytesToRead)
	prDebug("READ:  len=0x%x\n", toRead)
	n, err := w.Conn.Read(buf[:toRead])
	if err != nil || n == 0 {
		w.readClosed = true
		return n, err
	}
	w.bytesToRead -= n
	return n, nil
}

// Write a buffer
func (w *wrappedConn) Write(buf []byte) (int, error) {
	if w.writeClosed {
		return 0, ErrSocketWriteClosed
	}

	var err error
	toWrite := len(buf)
	written := 0

	prDebug("WRITE: Total len=%x\n", len(buf))

	for toWrite > 0 {
		if w.writeClosed {
			return 0, ErrSocketWriteClosed
		}

		// We write batches of MSG + data which need to be
		// "atomic". We don't want to hold the lock for the
		// entire Write() in case some other threads wants to
		// send OOB data, e.g. for closing.

		w.wrlock.Lock()

		thisBatch := min(toWrite, maxMsgSize)
		prDebug("WRITE: len=%x\n", thisBatch)
		// Write message header
		err = w.sendMsg(uint32(thisBatch))
		if err != nil {
			prDebug("Write MSG Error: %s\n", err)
			goto ErrOut
		}

		// Write data
		n, err := w.Conn.Write(buf[written : written+thisBatch])
		if err != nil {
			prDebug("Write Error 3\n")
			goto ErrOut
		}
		if n != thisBatch {
			prDebug("Write Error 4\n")
			err = ErrSocketNotEnoughData
			goto ErrOut
		}
		toWrite -= n
		written += n
		w.wrlock.Unlock()
	}

	return written, nil

ErrOut:
	w.wrlock.Unlock()
	w.writeClosed = true
	return 0, err
}

// SetDeadline(), SetReadDeadline(), and
// SetWriteDeadline() are OS specific.

// Send a message to the other end
// The Lock must be held to call this functions
func (w *wrappedConn) sendMsg(msg uint32) error {
	b := make([]byte, 4)

	binary.LittleEndian.PutUint32(b, msg)
	n, err := w.Conn.Write(b)
	if err != nil {
		prDebug("Write Error 1\n")
		return err
	}
	if n != len(b) {
		return ErrSocketMsgWrite
	}
	return nil
}

func prDebug(format string, args ...interface{}) {
	if Debug {
		log.Printf(format, args...)
	}
}
