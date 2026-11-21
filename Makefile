.PHONY: build test cpu cuda cuda-onnx vet native-ocr
build: cpu
cpu:
	go build ./cmd/instsum ./cmd/fake-ollama ./cmd/igx-eval ./cmd/igx-bench
cuda:
	docker build --target cuda -t instsum:cuda .
cuda-onnx:
	docker build --target cuda-onnx -t instsum:cuda-onnx .
native-ocr:
	go build -tags onnx ./cmd/instsum
test:
	go test ./...
vet:
	go vet ./...
