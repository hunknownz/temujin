.PHONY: build run clean dashboard

# Build React dashboard then Go binary (single binary output)
build: dashboard
	go build -o temujin ./cmd/temujin

# Build React dashboard and copy to Go embed dir
dashboard:
	cd dashboard && npm run build
	rm -rf internal/server/dist
	cp -r dashboard/dist internal/server/dist

run: build
	./temujin serve

# Dev mode: Vite dev server + Go backend (hot reload frontend)
dev:
	cd dashboard && npm run dev &
	go run ./cmd/temujin serve

clean:
	rm -f temujin
	rm -rf dashboard/dist internal/server/dist
