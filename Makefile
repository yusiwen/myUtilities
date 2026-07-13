NAME=mu
BINDIR=bin
VERSION=$(shell /usr/bin/git --no-pager describe --tags || echo "unknown version")
COMMIT_SHA=$(shell /usr/bin/git --no-pager rev-parse --short HEAD)
BUILDTIME=$(shell date -u)
FRONTEND_DIR=wol/frontend
ES_FRONTEND_DIR=es/frontend
MOCK_FRONTEND_DIR=mock/frontend
QRCODE_FRONTEND_DIR=qrcode/frontend
JARINFO_FRONTEND_DIR=jarinfo/frontend
CRYPTO_FRONTEND_DIR=crypto/frontend
THEME_PARTIAL=shared/frontend/theme-partial.html
FRONTEND_DIRS=wol es mock qrcode jarinfo crypto
GOBUILD=CGO_ENABLED=0 go build -trimpath -ldflags '-X "main.Version=$(VERSION)" \
		-X "main.CommitSHA=$(COMMIT_SHA)" \
		-X "main.BuildTime=$(BUILDTIME)" \
		-w -s -buildid='

PLATFORM_LIST = \
	darwin-arm64 \
	linux-amd64 \
	linux-arm64

default:
	CGO_ENABLED=0 go build -trimpath -o $(BINDIR)/$(NAME) .

.PHONY: inject-theme restore-theme

inject-theme:
	@for dir in $(FRONTEND_DIRS); do \
	  html="$$dir/frontend/index.html"; \
	  sed -i '/<!-- inject:theme -->/r $(THEME_PARTIAL)' "$$html"; \
	  sed -i '/<!-- inject:theme -->/d' "$$html"; \
	done
	@echo "Theme partial injected into all frontends"

restore-theme:
	git checkout -- $(foreach dir,$(FRONTEND_DIRS),$(dir)/frontend/index.html)
	@echo "Frontend index.html restored"

frontend: inject-theme
	@echo "Building WOL Svelte frontend..."
	cd $(FRONTEND_DIR) && npm install --silent && npm run build
	@echo '{"version": "$(VERSION)"}' > $(FRONTEND_DIR)/dist/version.json
	@echo "Building ES Svelte frontend..."
	cd $(ES_FRONTEND_DIR) && npm install --silent && npm run build
	@echo '{"version": "$(VERSION)"}' > $(ES_FRONTEND_DIR)/dist/version.json
	@echo "Building Mock Dynamic Svelte frontend..."
	cd $(MOCK_FRONTEND_DIR) && npm install --silent && npm run build
	@echo "Building QR Code Svelte frontend..."
	cd $(QRCODE_FRONTEND_DIR) && npm install --silent && npm run build
	@echo "Building JAR Info Svelte frontend..."
	cd $(JARINFO_FRONTEND_DIR) && npm install --silent && npm run build
	@echo "Building Crypto Svelte frontend..."
	cd $(CRYPTO_FRONTEND_DIR) && npm install --silent && npm run build
	$(MAKE) restore-theme

build: frontend default

WINDOWS_ARCH_LIST = \
	windows-amd64

all: frontend linux-amd64 linux-arm64 windows-amd64 darwin-arm64 # Most used

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

linux-arm64: frontend
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
