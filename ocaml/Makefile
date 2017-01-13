
.PHONY: build-in-container linux windows clean

PACKAGES=fmt.tty astring cstruct lwt lwt.unix hvsock hvsock.lwt

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
	ocamlopt -linkall $(OFLAGS) -g -o $@ $<

clean:
	rm -rf build
	docker image rm $(IMAGE)
	rm -f .$(IMAGE)
