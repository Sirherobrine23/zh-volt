FINAL_FILE ?= zhVolt
NETDEV ?= eth0
LOG_LEVEL ?= -4
LOG_FILE ?= debug.log

build:
	go build -v -o $(FINAL_FILE) .

clean:
	rm -f $(FINAL_FILE)

run: build
	sudo ./$(FINAL_FILE) daemon -v $(LOG_LEVEL) -L $(LOG_FILE) -i $(NETDEV)