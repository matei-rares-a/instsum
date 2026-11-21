FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /instsum ./cmd/instsum && \
    CGO_ENABLED=0 go build -o /igx-eval ./cmd/igx-eval && \
    CGO_ENABLED=0 go build -o /igx-bench ./cmd/igx-bench

FROM build AS test
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg tesseract-ocr tesseract-ocr-eng tesseract-ocr-ron && rm -rf /var/lib/apt/lists/*
RUN IGX_REQUIRE_MEDIA_TOOLS=1 go test -v -timeout 120s -count=1 ./... && go vet ./...

FROM debian:bookworm-slim AS cpu
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg tesseract-ocr tesseract-ocr-eng tesseract-ocr-ron python3 python3-pip python3-venv ca-certificates && \
    python3 -m venv /opt/tools && /opt/tools/bin/pip install --no-cache-dir yt-dlp==2026.10.26 gallery-dl==1.32.20 instaloader==4.16 && \
    rm -rf /var/lib/apt/lists/*
ENV PATH="/opt/tools/bin:${PATH}"
WORKDIR /app
COPY --from=build /instsum /usr/local/bin/instsum
COPY --from=build /igx-eval /usr/local/bin/igx-eval
COPY --from=build /igx-bench /usr/local/bin/igx-bench
COPY configs configs
COPY profiles profiles
COPY prompts prompts
COPY testdata/ollama_fixtures testdata/ollama_fixtures
ENTRYPOINT ["instsum"]

FROM nvidia/cuda:12.4.1-cudnn-devel-ubuntu22.04 AS native-build
ARG LLAMA_REF=b7766
ARG WHISPER_REF=v1.7.6
RUN apt-get update && apt-get install -y --no-install-recommends git cmake build-essential libcurl4-openssl-dev ca-certificates && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch ${LLAMA_REF} https://github.com/ggml-org/llama.cpp /llama && \
    cmake -S /llama -B /llama/build -DGGML_CUDA=ON -DBUILD_SHARED_LIBS=OFF -DLLAMA_CURL=ON && \
    cmake --build /llama/build --config Release -j2 --target llama-server
RUN git clone --depth 1 --branch ${WHISPER_REF} https://github.com/ggml-org/whisper.cpp /whisper && \
    cmake -S /whisper -B /whisper/build -DGGML_CUDA=ON -DBUILD_SHARED_LIBS=OFF && \
    cmake --build /whisper/build --config Release -j2 --target whisper-cli whisper-server

FROM golang:1.23-bullseye AS onnx-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags onnx -o /instsum-onnx ./cmd/instsum

FROM nvidia/cuda:12.4.1-cudnn-runtime-ubuntu22.04 AS cuda
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg tesseract-ocr tesseract-ocr-eng tesseract-ocr-ron python3 python3-pip python3-venv ca-certificates libgomp1 libcurl4 && \
    python3 -m venv /opt/tools && /opt/tools/bin/pip install --no-cache-dir yt-dlp==2026.10.26 gallery-dl==1.32.20 instaloader==4.16 && rm -rf /var/lib/apt/lists/*
ENV PATH="/opt/tools/bin:${PATH}"
WORKDIR /app
COPY --from=build /instsum /usr/local/bin/instsum
COPY --from=build /igx-eval /usr/local/bin/igx-eval
COPY --from=build /igx-bench /usr/local/bin/igx-bench
COPY --from=native-build /llama/build/bin/llama-server /usr/local/bin/llama-server
COPY --from=native-build /whisper/build/bin/whisper-cli /usr/local/bin/whisper-cli
COPY --from=native-build /whisper/build/bin/whisper-server /usr/local/bin/whisper-server
COPY configs configs
COPY profiles profiles
COPY prompts prompts
COPY testdata/ollama_fixtures testdata/ollama_fixtures
ENTRYPOINT ["instsum"]

FROM cuda AS cuda-onnx
ARG ONNX_RUNTIME_DIR=runtime/onnxruntime
COPY --from=onnx-build /instsum-onnx /usr/local/bin/instsum
COPY ${ONNX_RUNTIME_DIR}/lib/ /opt/onnxruntime/lib/
ENV LD_LIBRARY_PATH="/opt/onnxruntime/lib:${LD_LIBRARY_PATH}" \
    IGX_OCR_RUNTIME_PATH=/opt/onnxruntime/lib/libonnxruntime.so \
    IGX_OCR_ENGINE=paddle-onnx \
    IGX_OCR_DEVICE=gpu
