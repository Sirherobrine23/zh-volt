FINAL_FILE ?= zhVolt
NETDEV ?= eth0
VERBOSE ?= 7

build:
	go build -o $(FINAL_FILE) .

clean:
	rm -f $(FINAL_FILE)

run: build
	sudo ./$(FINAL_FILE) daemon -v $(VERBOSE) -i $(NETDEV)