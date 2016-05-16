package hvsock

import (
	"errors"
	"io"
	"log"
	"syscall"
	"time"
	"unsafe"
)

var (
	modws2_32   = syscall.NewLazyDLL("ws2_32.dll")

	procConnect     = modws2_32.NewProc("connect")
	procBind        = modws2_32.NewProc("bind")
	procRecv        = modws2_32.NewProc("recv")
	procSend        = modws2_32.NewProc("send")
	procCloseSocket = modws2_32.NewProc("closesocket")
)


// Make sure Winsock2 is initialised
func init() {
	e := syscall.WSAStartup(uint32(0x202), &wsaData)
	if e != nil {
		log.Fatal("WSAStartup", e)
	}
}

const (
	AF_HYPERV     = 34
	SHV_PROTO_RAW = 1
	socket_error  = uintptr(^uint32(0))
)

// struck sockaddr equivalent
type rawSockaddrHyperv struct {
	Family    uint16
	Reserved  uint16
	VmId      GUID
	ServiceId GUID
}

type hvsockListener struct {
	accept_fd syscall.Handle
	laddr     HypervAddr
}

// Internal representation. Complex mostly due to asynch send()/recv() syscalls.
type hvsockConn struct {
	fd     syscall.Handle
	local  HypervAddr
	remote HypervAddr
}


var (
	wsaData syscall.WSAData
)

// Main constructor

// Utility function to build a struct sockaddr for syscalls.
func (a HypervAddr) sockaddr(sa *rawSockaddrHyperv) (unsafe.Pointer, int32, error) {
	sa.Family = AF_HYPERV
	sa.Reserved = 0
	for i := 0; i < len(sa.VmId); i++ {
		sa.VmId[i] = a.VmId[i]
	}
	for i := 0; i < len(sa.ServiceId); i++ {
		sa.ServiceId[i] = a.ServiceId[i]
	}

	return unsafe.Pointer(sa), int32(unsafe.Sizeof(*sa)), nil
}

func connect(s syscall.Handle, a *HypervAddr) (err error) {
	var sa rawSockaddrHyperv
	ptr, n, err := a.sockaddr(&sa)
	if err != nil {
		return err
	}

	r1, _, e1 := syscall.Syscall(procConnect.Addr(), 3, uintptr(s), uintptr(ptr), uintptr(n))
	if r1 == socket_error {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return nil
}

func bind(s syscall.Handle, a HypervAddr) error {
	var sa rawSockaddrHyperv
	ptr, n, err := a.sockaddr(&sa)
	if err != nil {
		return err
	}

	r1, _, e1 := syscall.Syscall(procBind.Addr(), 3, uintptr(s), uintptr(ptr), uintptr(n))
	if r1 == socket_error {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return nil
}

// XXX Untested
func accept(s syscall.Handle, a *HypervAddr) (syscall.Handle, error) {
	return 0, errors.New("accept(): Unimplemented")
}

//
// File IO/Socket interface
//
func newHVsockConn(h syscall.Handle, local HypervAddr, remote HypervAddr) (*HVsockConn, error) {
	v := &hvsockConn{fd: h, local: local, remote: remote}
	return &HVsockConn{hvsockConn: *v}, nil
}

func (v *HVsockConn) close() (err error) {
	r1, _, e1 := syscall.Syscall(procCloseSocket.Addr(), 1, uintptr(v.fd), 0, 0)
	if r1 == socket_error {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	v.fd = syscall.InvalidHandle
	return err
}

// Underlying raw read() function.
func (v *HVsockConn) read(buf []byte) (int, error) {
	var err error
	ptr := unsafe.Pointer(&buf[0])
	n := uint32(len(buf))
	r1, _, e1 := syscall.Syscall(procRecv.Addr(), 3, uintptr(v.fd), uintptr(ptr), uintptr(n))
	if r1 == socket_error {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}

	// Handle EOF conditions.
	if err == nil && r1 == 0 && len(buf) != 0 {
		return 0, io.EOF
	} else if err == syscall.ERROR_BROKEN_PIPE {
		return 0, io.EOF
	}
	return int(r1), err
}

// Underlying raw write() function.
func (v *HVsockConn) write(buf []byte) (int, error) {
	var err error

	if len(buf) == 0 {
		return 0, nil
	}

	ptr := unsafe.Pointer(&buf[0])
	n := uint32(len(buf))
	r1, _, e1 := syscall.Syscall(procSend.Addr(), 3, uintptr(v.fd), uintptr(ptr), uintptr(n))
	if r1 == socket_error {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return int(r1), err
}

func (v *HVsockConn) SetReadDeadline(t time.Time) error {
	return nil // FIXME
}

func (v *HVsockConn) SetWriteDeadline(t time.Time) error {
	return nil // FIXME
}

func (v *HVsockConn) SetDeadline(t time.Time) error {
	return nil // FIXME
}
