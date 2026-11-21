package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

	"instsum/internal/vlm/ollama/fakeserver"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:11435", "listen address")
	model := flag.String("model", "qwen2.5vl:7b", "installed model")
	mode := flag.String("mode", "", "missing, timeout, invalid-json, server-down, error, chat-error, pull-error, missing-pull-error or unload-error")
	response := flag.String("response", `{"ok":true}`, "chat content JSON")
	capabilities := flag.String("capabilities", "completion,vision", "comma-separated /api/show model capabilities")
	contextLength := flag.Int("context", 8192, "reported model context length")
	flag.Parse()
	server := fakeserver.Server{
		Model: *model, Mode: *mode, Response: *response,
		Capabilities: strings.Split(*capabilities, ","), ContextLength: *contextLength,
	}
	log.Fatal(http.ListenAndServe(*addr, server))
}
