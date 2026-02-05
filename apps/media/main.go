package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/pion/webrtc/v3"
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
	r.Body = http.MaxBytesReader(w, r.Body, 5000)
	offer, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("Received SDP Offer (%d bytes)", len(offer))

	// 6. MediaEngine 설정 (코덱 명시적 등록)
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		http.Error(w, "Content-Type must be application/sdp", http.StatusUnsupportedMediaType)
		return
	}

	// 7. SettingEngine 설정
	settingEngine := webrtc.SettingEngine{}
	if err := settingEngine.SetEphemeralUDPPortRange(50000, 50050); err != nil {
		http.Error(w, "Failed to set UDP port range", http.StatusInternalServerError)
		log.Printf("UDP port range set failed: %v", err)
		return
	}

	// 8. API 객체 생성 (MediaEngine + SettingEngine)
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	)

	// 9. ICE 서버 설정 (STUN)
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// 10. PeerConnection 생성
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		http.Error(w, "Failed to create PeerConnection", http.StatusInternalServerError)
		log.Printf("PeerConnection creation failed: %v", err)
		return
	}

	// 11. Remote Description 설정
	if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(offer),
	}); err != nil {
		http.Error(w, "Failed to set remote description", http.StatusBadRequest)
		log.Printf("SetRemoteDescription failed: %v", err)
		peerConnection.Close()
		return
	}

	log.Println("PeerConnection created and remote description set")

	answer := "TODO: Generate SDP Answer via CreateAnswer"

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/whip/session-id")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, answer)

	log.Println("WHIP handler completed (answer placeholder)")
}
