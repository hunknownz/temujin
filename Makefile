.PHONY: build run clean

build:
	go build -o temujin ./cmd/temujin

run: build
	./temujin serve

clean:
	rm -f temujin
