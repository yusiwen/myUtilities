NAME=mu
BINDIR=bin
VERSION=$(shell git describe --tags || echo "unknown version")
BUILDTIME=$(shell date -u)
FRONTEND_DIR=wol/frontend
GOBUILD=CGO_ENABLED=0 go build -trimpath -ldflags '-X "main.Version=$(VERSION)" \
		-X "main.CommitSHA=$(shell git rev-parse --short HEAD)" \
		-X "main.BuildTime=$(BUILDTIME)" \
		-w -s -buildid='

PLATFORM_LIST = \
	darwin-arm64 \
	linux-amd64 \
	linux-armv8

frontend:
	@echo "Building Svelte frontend..."
	cd $(FRONTEND_DIR) && npm install --silent && npm run build

WINDOWS_ARCH_LIST = \
	windows-amd64

all: frontend linux-amd64 linux-armv8 windows-amd64 darwin-arm64 # Most used

docker: frontend
	$(GOBUILD) -o $(BINDIR)/$(NAME)-$@

darwin-amd64: frontend
	GOARCH=amd64 GOOS=darwin $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

darwin-arm64: frontend
	GOARCH=arm64 GOOS=darwin $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-386: frontend
	GOARCH=386 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-amd64: frontend
	GOARCH=amd64 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv5: frontend
	GOARCH=arm GOOS=linux GOARM=5 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv6: frontend
	GOARCH=arm GOOS=linux GOARM=6 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv7: frontend
	GOARCH=arm GOOS=linux GOARM=7 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv8: frontend
	GOARCH=arm64 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-mips-softfloat: frontend
	GOARCH=mips GOMIPS=softfloat GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-mips-hardfloat: frontend
	GOARCH=mips GOMIPS=hardfloat GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-mipsle-softfloat: frontend
	GOARCH=mipsle GOMIPS=softfloat GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-mipsle-hardfloat: frontend
	GOARCH=mipsle GOMIPS=hardfloat GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-mips64: frontend
	GOARCH=mips64 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-mips64le: frontend
	GOARCH=mips64le GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

freebsd-386: frontend
	GOARCH=386 GOOS=freebsd $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

freebsd-amd64: frontend
	GOARCH=amd64 GOOS=freebsd $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

freebsd-arm64: frontend
	GOARCH=arm64 GOOS=freebsd $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

windows-386: frontend
	GOARCH=386 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-amd64: frontend
	GOARCH=amd64 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-arm64: frontend
	GOARCH=arm64 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-arm32v7: frontend
	GOARCH=arm GOOS=windows GOARM=7 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

gz_releases=$(addsuffix .gz, $(PLATFORM_LIST))
zip_releases=$(addsuffix .zip, $(WINDOWS_ARCH_LIST))

$(gz_releases): %.gz : %
	chmod +x $(BINDIR)/$(NAME)-$(basename $@)
	gzip -f -S -$(VERSION).gz $(BINDIR)/$(NAME)-$(basename $@)

$(zip_releases): %.zip : %
	zip -m -j $(BINDIR)/$(NAME)-$(basename $@)-$(VERSION).zip $(BINDIR)/$(NAME)-$(basename $@).exe

all-arch: $(PLATFORM_LIST) $(WINDOWS_ARCH_LIST)

releases: $(gz_releases) $(zip_releases)
clean:
	rm $(BINDIR)/*
