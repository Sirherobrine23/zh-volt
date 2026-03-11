NETDEV ?= eth0

build: fmt
	cargo build

release: fmt
	cargo build --release

fmt:
	cargo fmt

clean:
	rm -f ./target

run: build
	sudo RUST_BACKTRACE=1 ./target/debug/zh_volt -i $(NETDEV)