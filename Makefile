bin:
	mkdir -p bin
	go build -o bin/edistribucion-store ./cmd/edistribucion-store
	go build -o bin/contadores ./cmd/edistribucion

.PHONY: bin
