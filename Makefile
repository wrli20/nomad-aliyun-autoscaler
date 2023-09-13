GO_LDFLAGS := "$(GO_LDFLAGS) -X github.com/wrli20/nomad-aliyun-autoscaler/version.GitCommit=$(GIT_COMMIT)$(GIT_DIRTY)"
OS := "linux"
ARCH := "amd64"

.PHONY: plugins
bin/plugins/acs-ess:
	@echo "==> Building $@..."
	@CGO_ENABLED=0 \
	GOOS=$(OS) \
	GOARCH=$(ARCH) \
 	go build \
	-ldflags $(GO_LDFLAGS) \
	-o ./bin/acs-ess
	@echo "==> Done"