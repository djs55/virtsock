
.PHONY: build-in-container linux windows depends clean

PACKAGES=mtime mtime.os cstruct cstruct.lwt lwt lwt.unix lwt.preemptive \
  hvsock hvsock.lwt hvsock.lwt-unix logs logs.fmt

OFLAGS=$(shell \
        ocamlfind query $(PACKAGES) -r \
        -format "-I %d %a") -w A-4-41-45-44 -warn-error +1..49

IMAGE=ocaml-hvsock-build

# Build Linux binaries in a container
build-in-container: .$(IMAGE)
	mkdir -p build
	docker run --rm -v ${CURDIR}:/src $(IMAGE) opam config exec -- make linux

.$(IMAGE): opam Dockerfile.build
	docker image rm $(IMAGE) || true
	docker build -t $(IMAGE) -f ./Dockerfile.build .
	touch .$(IMAGE)

linux: build/hvbench
windows: build/hvbench.exe

build/hvbench: hvbench.ml
	mkdir -p build
	ocamlfind ocamlopt -package "$(PACKAGES)" -thread -linkpkg $(OFLAGS) -g -o $@ $<

build/hvbench.exe: hvbench.ml
	mkdir -p build
	ocamlfind ocamlopt -package "$(PACKAGES)" -thread -linkpkg $(OFLAGS) -g -o $@ $<

depends:
	opam pin add hvtools . -n
	opam depext hvtools -y
	opam install hvtools --deps-only

clean:
	rm -rf build
	docker image rm $(IMAGE)
	rm -f .$(IMAGE)
