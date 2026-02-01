package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/whip", whipHandler)

	log.Println("Media Server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

// whipHandler handles WHIP protocol (WebRTC-HTTP Ingestion Protocol)
// WHIP is simply: POST /whip with SDP Offer → Return SDP Answer
func whipHandler(w http.ResponseWriter, r *http.Request) {
	// 1. CORS 헤더 설정
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// 2. Preflight 처리
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 3. 메서드 검증
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 4. 미디어 타입 검증
	if r.Header.Get("Content-Type") != "application/sdp" {
		http.Error(w, "Content-Type must be application/sdp", http.StatusUnsupportedMediaType)
		return
	}

	// 5. SDP Offer 읽기
	offer, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("Received SDP Offer (%d bytes)", len(offer))

	// TODO: Generate SDP Answer via WebRTC PeerConnection
	answer := "TODO: Generate SDP Answer via WebRTC PeerConnection"

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/whip/session-id")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, answer)

	log.Println("WHIP handler completed (answer placeholder)")
}
