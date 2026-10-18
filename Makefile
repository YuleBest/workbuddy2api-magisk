.PHONY: test build clean

VERSION := $(shell sed -n 's/^version=//p' module/module.prop | head -n 1)
ZIP := build/wb2api-$(VERSION)-arm64-v8a.zip
MODULE_VALIDATOR ?= /home/yule/.agents/skills/magisk-kernelsu-module/scripts/validate_module.py

test:
	go test ./...
	go vet ./...
	shellcheck module/*.sh scripts/build_android.sh scripts/test_customize.sh
	shfmt -i 4 -d module/*.sh scripts/build_android.sh scripts/test_customize.sh
	./scripts/test_customize.sh
	@if [ -f "$(MODULE_VALIDATOR)" ]; then \
		python3 "$(MODULE_VALIDATOR)" build/stage; \
		python3 "$(MODULE_VALIDATOR)" "$(ZIP)"; \
	else \
		printf 'Module validator not found, skipping: %s\n' "$(MODULE_VALIDATOR)"; \
	fi

build:
	./scripts/build_android.sh

clean:
	rm -rf build
