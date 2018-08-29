This is a test to check whether the `close` syscall drops in-flight data.

The usual Linux socket (and pipe and file descriptor) behaviour is that
`close` will not drop in-flight data, but instead will push an EOF to
the other side.

This test case is intended to check whether calling `close` on a Hyper-V
socket in a Linux VM has the same behviour.

# Pre-requesites

## Build the iso

The test case requires a Linux or Mac machine to build the `hvtest-efi.iso`
with LinuxKit. Run `make linuxkit` in the root of this repo to build the iso
and then copy it to the WIndows test machine.

## Build the test program

The test case runs a server in a Linux VM and a client on a Windows host.
Build the client on the Windows host using the Go toolchain:
```
cd cmd/close_test
go build
```

## Register the Hyper-V socket GUID

Next register the Hyper-V socket GUID in the registry. Follow the instructions on [making an integration service](https://docs.microsoft.com/en-gb/virtualization/hyper-v-on-windows/user-guide/make-integration-service):

- create a new registry key (e.g. with `regedit.exe`) called `3049197C-FACB-11E6-BD58-64006A7986D3` under `HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Virtualization\GuestCommunicationServices`
- inside the new key create a String value called `ElementName`. The Data can be anything you like but typically this is a description such as "sock_stress".

# Running the test

In an elevated powershell on the Windows machine:

```
linuxkit run hvtest-efi.iso
```

In the VM console run

```
/usr/bin/close_test -s hvsock
```

In another elevated powershell on the Windows machine:

First query the VM's GUID:
```
(get-vm hvtest).Id
```

Next tell the client to connect to the server with:
```
./close_test.exe -c hvsock://<GUID>
```

