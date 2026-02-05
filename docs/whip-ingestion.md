# WHIP Ingestion

## 1. HTTP 핸들러 구현

WebRTC는 연결 방식은 표준화되어 있지만, 정작 연결 정보를 주고받는 시그널링 방식은 오랫동안 제각각이었다. 이 혼란을 정리하기 위해 등장한 표준이 바로 **WHIP (WebRTC-HTTP Ingestion Protocol)** 이다.

작동 원리는 아주 단순하다.

> "HTTP POST로 SDP(연결 정보)를 보내고, 응답으로 서버의 SDP를 받는다."

이 단순한 표준 덕분에 우리는 더 이상 복잡한 독자 규격을 설계할 필요가 없다. 서버가 WHIP만 지원하면 OBS Studio 같은 범용 도구에서 별도 설정 없이 바로 우리 서버로 방송을 송출할 수 있기 때문이다.

이제 이 통로를 지탱할 HTTP 핸들러를 직접 구현해 보자.

### 1.1 SOP와 CORS

브라우저는 신뢰할 수 없는 스크립트(JS)를 실행하는 환경이다. 만약 `evil.com`에 접속했는데, 이 사이트의 스크립트가 내 브라우저에 저장된 인증 정보를 이용해 `bank.com`에서 내 돈을 인출하려 한다면 어떻게 될까?

이런 정보 유출을 방지하기 위해 브라우저는 **SOP(Same-Origin Policy)**를 따른다. 출처(Origin)란 프로토콜, 도메인, 포트 번호를 합친 식별자다. 브라우저는 이 출처가 다르면 리소스에 대한 접근을 금지한다. SOP는 기본적으로 요청 전송(Write)을 막는 게 아니라, 응답 읽기(Read)를 막는다.

1. Write: 브라우저는 일단 요청을 서버로 보낸다. 서버는 이 요청을 정상적으로 처리하고 답장까지 돌려준다.
2. Read: 브라우저는 응답을 받은 뒤 헤더를 검사한다. 만약 서버의 명시적인 허가가 없다면, JS(fetch)가 응답 내용을 읽지 못하도록 데이터를 폐기해 버린다.

하지만 우리의 프로젝트에서는 클라이언트(`localhost:5173`)와 미디어 서버(`localhost:8080`)의 포트가 달라 서로 다른 출처에 해당한다. 정상적인 통신을 위해서는 서버가 브라우저에게 "이 출처는 내가 검증했으니 응답을 보여줘도 좋다"라고 알려줘야 한다. 이 메커니즘이 바로 **CORS(Cross-Origin Resource Sharing)**다.

```go
func whipHandler(w http.ResponseWriter, r *http.Request) {
    // 1. CORS 헤더 설정
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
    // ...
}
```

### 1.2 Preflight Request

앞선 설명에서 브라우저는 일단 요청을 보내고, 응답만 가린다고 했다. 만약 그 요청이 데이터를 삭제하는 `DELETE` 요청이었다면? 브라우저가 응답을 가려봤자, 서버에서는 이미 데이터가 삭제된(Side Effect) 후가 아닐까? 맞다. 그래서 브라우저는 요청을 두 가지 종류로 나눈다.

- **Simple Request:** `form-urlencoded`와 같은 오래된 방식으로, 하위 호환성을 위해 preflight를 사용하지 않는다. (대신 CSRF 토큰 등으로 보호해야 한다.)
- **Complex Request:** `application/json`이나 `application/sdp`와 같은 현대적인 방식이다.

Complex Request의 경우, 브라우저는 무턱대고 본 요청을 보내지 않는다. 대신 **Preflight** 라는 안전장치를 가동한다. 브라우저는 본 요청을 보내기 전, OPTIONS 메서드로 서버에게 먼저 묻는다.

> "`application/sdp` 타입으로 `POST` 보낼 건데, 받아줄 거야?"

이 질문에 대한 서버의 반응에 따라 브라우저의 행동이 갈린다.

- **허가:** 서버가 `200 OK`와 함께 허가 헤더를 내려주면, 브라우저는 대기 중이던 본 요청(`POST`)을 전송한다.
- **거부:** 서버가 거절하거나 응답하지 않으면, 브라우저는 본 요청을 전송하지 않는다.

우리가 사용할 WHIP 프로토콜은 `application/sdp`를 사용하므로 브라우저가 자동으로 Preflight를 수행한다. 따라서 서버 코드에서는 `OPTIONS` 메서드에 대한 처리를 가장 먼저 해줘야 한다. 그렇지 않으면 브라우저는 연결을 시도하지 않을 것이다.

```go
func whipHandler(w http.ResponseWriter, r *http.Request) {
    // ...
    // 2. Preflight 처리
    if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
    // ...
}
```

### 1.3 HTTP 메서드와 Content-Type 검증

[WHIP 규약(RFC 9725)](https://datatracker.ietf.org/doc/rfc9725)는 세션 생성에 대해 다음과 같은 가이드라인을 제시한다.

1. 메서드: `POST`
2. Content-Type: `application/sdp`

우리는 표준대로 POST가 맞는지 확인하고, preflight 보안 장치가 작동하도록 타입이 맞는지 순서대로 확인하면 된다.

```go
func whipHandler(w http.ResponseWriter, r *http.Request) {
    // ...
    // 3. 메서드 검증
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // 4. 미디어 타입 검증
    // WHIP spec: application/sdp 필수
    // (공격자가 text/plain 등으로 위장하여 Preflight를 우회하는 것을 차단)
    if r.Header.Get("Content-Type") != "application/sdp" {
        http.Error(w, "Content-Type must be application/sdp", http.StatusUnsupportedMediaType)
        return
    }
    // ...
}
```

### 1.4 SDP 교환

검문(CORS, Preflight, Validation)을 모두 통과했다면, 서버는 브라우저의 **SDP(Session Description Protocol)**를 읽는다.

SDP는 미디어 데이터 자체가 아니다. "나는 H.264 코덱을 쓸 거고, 해상도는 얼마다" 같은 정보가 담긴 텍스트다. 실제 미디어(오디오/비디오)는 연결 수립 후 RTP 프로토콜로 전송된다.

WebRTC 스트리밍에서 사용되는 SDP 필드는 `m=` (미디어 타입), `a=rtpmap` (코덱), `a=candidate` (네트워크 경로), `a=fingerprint` (보안) 정도인데, 직접 파싱할 필요는 없고 `pion/webrtc`와 같은 라이브러리를 사용하면 된다. 더 자세한 내용은 [정보통신기술용어해설](http://www.ktword.co.kr/test/view/view.php?no=2114)을 읽어보자.

서버는 SDP offer를 읽어서(`io.ReadAll`) 자신이 지원할 수 있는지 판단하고, 자신의 정보가 담긴 SDP answer를 돌려준다. 여기서 WHIP의 요구사항 두 가지를 구현한다.

1. **201 Created:** 방송 세션이라는 '리소스'가 서버에 새로 생성되었으므로, 일반적인 200 OK가 아닌 201 Created를 반환한다.
2. **Location 헤더:** 서버는 생성된 세션을 제어할 수 있는 고유 주소(Resource URL)를 알려줘야 한다. 클라이언트는 나중에 방송을 종료(DELETE)하거나 상태를 변경(PATCH)할 때 이 주소를 사용하게 된다.

```go
func whipHandler(w http.ResponseWriter, r *http.Request) {
    // ...
    // 5. SDP Offer 읽기
    offer, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Failed to read body", http.StatusBadRequest)
        return
    }
    defer r.Body.Close()

    log.Printf("Received SDP Offer (%d bytes)", len(offer))
    
    answer := "TODO: Generate SDP Answer via WebRTC PeerConnection"

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/whip/session-id") // 우선 하드코딩 했지만, 고유한 세션 ID를 사용해야 한다.
	w.WriteHeader(http.StatusCreated)
    fmt.Fprint(w, answer) // SDP answer 전송

	log.Println("WHIP handler completed (answer placeholder)")
}
```

### 1.5 테스트

이제 작성한 코드가 의도대로 작동하는지 테스트해 보자.

```bash
go run main.go
```

#### Preflight 테스트

브라우저가 보내는 예비 요청(`OPTIONS`)을 흉내 내어 본다. 서버는 `Access-Control-Allow-Origin` 헤더를 포함한 `HTTP/1.1 200 OK`를 반환해야 한다.

```bash
curl -i -X OPTIONS http://localhost:8080/whip \
  -H "Access-Control-Request-Method: POST"
```

#### Content-Type 테스트

`Content-Type: application/sdp` 헤더 없이(또는 틀린 타입으로) 요청을 보내면, 서버는 `415 Unsupported Media Type` 에러를 반환해야 한다.

```bash
# text/plain으로 보내면 거부되어야 함
curl -i -X POST http://localhost:8080/whip \
  -H "Content-Type: text/plain" \
  -d "fake data"
```

#### 정상 동작 테스트

마지막으로, WHIP 표준에 맞게 헤더와 바디를 갖춰서 요청을 보내보자. 서버는 `201 Created` 응답과 함께, 방송을 제어할 수 있는 `Location` 헤더를 반환해야 한다.

```bash
curl -i -X POST http://localhost:8080/whip \
  -H "Content-Type: application/sdp" \
  -d "v=0..."
```

기본적인 HTTP 핸들러 구현이 완료되었다. 다음 단계로 **WebRTC PeerConnection**를 활용해, 실제 SDP를 주고받는 과정을 구현해 보자.

---

## 2. PeerConnection 생성

HTTP 핸들러라는 '문'을 만들었으니, 이제 그 안에서 실제로 미디어를 처리할 '엔진'을 돌릴 차례다. WebRTC의 핵심 객체인 **PeerConnection**을 생성하고, 클라이언트와 협상하는 과정을 단계별로 구현해 보자.

### 2.1 MediaEngine: 코덱의 언어 맞추기

WebRTC 연결의 첫 단추는 코덱 협상(Codec Negotiation)이다. 클라이언트와 서버가 서로 어떤 영상/음성 포맷을 쓸지 합의해야 하는데, 이를 관리하는 것이 MediaEngine이다.

서버가 지원하는 코덱 목록을 명시적으로 등록하지 않으면, 코덱 불일치로 인해 연결이 즉시 실패할 수 있다. 이는 WebRTC의 표준인 [JSEP(RFC 8829)](https://datatracker.ietf.org/doc/html/rfc8829#section-3.2)의 협상 방식 때문이다.

WebRTC 협상은 양측이 제시한 코덱 목록의 교집합을 찾는 과정이다. 서버의 MediaEngine이 비어 있다면, 클라이언트가 무엇을 제안하더라도 서버가 이를 처리할 수 없다고 판단하여 미디어 전송 자체가 거부된다.

또한 WebRTC의 코덱 협상은 단순히 코덱 이름(H.264 등)만 일치한다고 성립하지 않는다. profile-level-id나 packetization-mode 같은 세부 파라미터까지 정확히 일치해야 동일한 코덱으로 간주된다.

마지막으로, Pion 라이브러리는 명시적 등록 방식을 지향한다. "기본값으로 모든 코덱을 활성화"하면 불필요한 처리 로직이 시스템 자원을 낭비할 수 있기 때문에, 실제 사용할 코덱만 등록하여 효율적인 미디어 파이프라인을 구성한다.

```go
    // 6. MediaEngine 설정 (코덱 등록)
    mediaEngine := &webrtc.MediaEngine{}
    if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
        http.Error(w, "Failed to register codecs", http.StatusInternalServerError)
        return
    }
```

### 2.2 SettingEngine

MediaEngine이 무엇(What)을 지원할지 정의한다면, SettingEngine은 그 기능들이 실제 네트워크 환경에서 어떻게(How) 동작할지 정의한다.

이 서버는 Docker 컨테이너 위에서 실행할 예정인데, WebRTC는 미디어 전송을 위해 동적으로 선택된 UDP 포트를 사용한다. 반면 Docker는 컨테이너 외부에서 접근할 수 있는 포트를 사전에 명시적으로 개방(Port Forwarding) 해야 한다.

물론 넓은 범위의 포트를 모두 개방할 수도 있지만 이는 방화벽 규칙 관리를 복잡하게 만들고 보안상 위험하다. 따라서 **SettingEngine**을 사용해 WebRTC가 사용할 UDP 포트의 범위를 제한할 수 있다.

이제 `MediaEngine`과 `SettingEngine`을 하나로 묶어 WebRTC API 객체를 생성할 수 있다. Pion은 하나의 프로세스 안에서도 서로 다른 설정을 가진 API 인스턴스를 여러 개 생성할 수 있도록 설계되어 있다. 이 API 객체를 통해 생성되는 모든 PeerConnection은 우리가 설정한 코덱과 포트 규칙을 따르게 된다.

```go
    // 7. SettingEngine 설정
    settingEngine := webrtc.SettingEngine{}
    if err := settingEngine.SetEphemeralUDPPortRange(50000, 50050); err != nil {
        http.Error(w, "Failed to set UDP port range", http.StatusInternalServerError)
        return
    }

    // 8. API 객체 생성
    api := webrtc.NewAPI(
        webrtc.WithMediaEngine(mediaEngine),
        webrtc.WithSettingEngine(settingEngine),
    )
```

### 2.3 API 객체 및 ICE 설정

WebRTC에서 서로 다른 네트워크에 있는 피어가 통신하려면, 먼저 서로에게 도달 가능한 주소(candidate)를 알아내야 한다. 이 과정은 ICE(Interactive Connectivity Establishment)의 첫 단계로, 각 피어는 자신에게 도달 가능한 여러 주소를 수집하고 교환한다.

NAT 뒤에 있는 호스트는 자신의 공인 IP와 포트를 직접 알 수 없다. NAT가 내부 호스트의 주소를 외부 주소로 변환하면서 그 정보를 내부 프로그램에 알려주지 않기 때문이다. 특히 모바일 네트워크나 클라우드 환경에서는 여러 단계의 NAT를 거치기도 한다. 따라서 우리의 서버와 같은 애플리케이션이 자신의 외부 주소를 직접 예측하는 것은 불가능에 가깝다.

이 문제를 해결하기 위해 STUN 서버를 사용한다. 서버는 STUN 서버에 요청을 보내 "외부에서 보이는 나의 공인 주소는 무엇인가?"를 확인하고, 이 정보를 기반으로 클라이언트와 연결 경로를 찾는다.

이제 서버가 자신의 공인 주소를 확인할 수 있도록 STUN 서버 정보를 ICE 설정에 추가한다. 이 설정은 ICE candidate gathering 과정에서 사용되며, 이후 생성되는 PeerConnection은 이 STUN 서버를 이용해 외부에서 도달 가능한 주소를 수집하게 된다.

*참고: 참고로 WHIP은 P2P가 아닌 클라이언트-서버 구조이고 서버의 포트(50000-50050)가 개방되어 있으므로, TURN 서버는 구성하지 않는다.*

```go
    // 9. ICE 서버 설정 (STUN)
    // 편의를 위해 Google에서 제공하는 공개 STUN 서버를 사용한다.
    config := webrtc.Configuration{
        ICEServers: []webrtc.ICEServer{
            {
                URLs: []string{"stun:stun.l.google.com:19302"},
            },
		},
	}
```

### 2.4 PeerConnection 생성 및 Offer 적용

드디어 **PeerConnection**을 생성하고, 클라이언트가 보낸 SDP Offer를 적용할 차례다.

`SetRemoteDescription`을 호출하면 서버는 SDP 문자열을 파싱하여 상대 피어가 지원하는 코덱, 미디어 설정, 네트워크 경로 정보를 파악하고 내부적으로 연결 준비를 시작한다.

> **주의:** 여기서 `defer peerConnection.Close()`를 호출하면 안 된다. HTTP 요청 처리가 끝나더라도 WebRTC 연결은 이후에도 계속 유지되어야 하기 때문이다. 연결 종료는 추후 `ConnectionStateChange` 이벤트 등에서 명시적으로 처리한다.

```go
    // 10. PeerConnection 생성
    peerConnection, err := api.NewPeerConnection(config)
    if err != nil {
        http.Error(w, "Failed to create PeerConnection", http.StatusInternalServerError)
        log.Printf("PeerConnection creation failed: %v", err)
        return
    }

    // 11. Remote Description 설정 (SDP Offer 적용)
    // 클라이언트가 보낸 SDP를 서버에 적용한다.
    // 이 과정에서 코덱 협상과 ICE 절차가 시작될 준비가 이루어진다.
    if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{
        Type: webrtc.SDPTypeOffer,
        SDP:  string(offer),
    }); err != nil {
        http.Error(w, "Failed to set remote description", http.StatusBadRequest)
        log.Printf("SetRemoteDescription failed: %v", err)
        peerConnection.Close() // 에러 발생 시 리소스 정리
        return
    }

    log.Println("PeerConnection created and remote description set")
```

### 2.5 테스트

작성한 WebRTC 엔진이 정상적으로 시동이 걸리는지 확인해 보자. 이번 테스트의 핵심은 **"서버가 클라이언트의 SDP Offer를 거부하지 않고 `SetRemoteDescription`을 성공하느냐"** 이다.

먼저 서버를 실행한다.

```bash
go run main.go
```

#### SDP Offer 시뮬레이션

WebRTC 연결을 맺으려면 문법에 맞는 SDP가 필요하다. 아래는 VP8 비디오 코덱을 사용하겠다고 선언하는 최소한의 SDP 예시다. 이 내용을 `curl`을 이용해 서버로 전송해 보자.

서버가 `201 Created`를 반환했다면, 다음 세 가지가 모두 정상 작동했음을 의미한다.

1. `MediaEngine`이 정상적으로 초기화되어 VP8 코덱을 인식했다.
2. `SettingEngine`이 포트 범위를 문제없이 설정했다.
3. 클라이언트가 보낸 SDP가 서버 설정과 일치(교집합)하여 `SetRemoteDescription`을 통과했다.

```bash
curl -i -X POST http://localhost:8080/whip \
  -H "Content-Type: application/sdp" \
  --data-binary "v=0
o=- 0 0 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=video 9 UDP/TLS/RTP/SAVPF 96
a=mid:0
a=sendonly
a=setup:actpass
a=ice-ufrag:testiceufrag
a=ice-pwd:testicepwd
a=rtpmap:96 VP8/90000
a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00
"
```

여기까지 진행했다면 서버는 클라이언트의 제안을 받아들일 준비를 마쳤다. 하지만 아직 우리의 "답장(Answer)"을 보내지 않았기에 연결은 대기 상태다. 이제 다음 단계에서 SDP Answer 생성과 실제 미디어 트랙 처리를 시작해 보자.

---

## 3. SDP Answer: 협상과 응답

지금까지 서버는 클라이언트가 보낸 SDP Offer를 받아들여 `RemoteDescription`으로 설정했다. 이는 "상대방이 무엇을 원하는지"를 이해한 상태다. 이제 서버는 "그래서 내가 무엇을 줄 수 있는지"를 담은 **SDP Answer**를 생성하여 응답해야 한다.

이 과정에서 우리는 두 가지 일을 한다.

1. **Media Negotiation:** 상대방의 제안 중 내가 처리 가능한 것을 선택(Intersection)한다.
2. **ICE Strategy:** 비동기적으로 수집되는 네트워크 후보(Candidate)들을 클라이언트에게 보낸다.

### 3.1 CreateAnswer: 교집합 찾기

WebRTC 연결은 서로 지원하는 코덱과 설정이 일치해야 성립된다. CreateAnswer 메서드는 앞서 우리가 MediaEngine에 등록한 코덱 목록(Server Capabilities)과 클라이언트의 Offer(Client Capabilities)를 비교하여 교집합을 찾아낸다.

여기서 주의할 점은 교집합이 하나도 없을 때의 동작이다. Pion 라이브러리는 교집합이 없다고 해서 에러를 반환하지 않는다(Soft Fail). API 호출 자체는 성공하지만, 생성된 SDP 내부의 미디어 포트가 0으로 설정된다(예: m=video 0 ...).

이는 "연결(Signaling)은 유지하되, 네가 제안한 미디어 트랙은 받지 않겠다(Rejected)"는 뜻이다. 개발자 입장에서는 에러 로그가 찍히지 않는데 영상은 들어오지 않는 '좀비 연결' 상태가 될 수 있으므로, 초기 개발 시 이 코덱 협상 과정을 주의 깊게 살펴야 한다.

```go
    // 12. SDP Answer 생성
    // 내부적으로 Offer와 MediaEngine의 교집합을 찾아 Answer를 만든다.
    answer, err := peerConnection.CreateAnswer(nil)
    if err != nil {
        http.Error(w, "Failed to create answer", http.StatusInternalServerError)
        return
    }
```

### 3.2 Trickle ICE vs Full ICE

표준 WebRTC는 연결 속도를 높이기 위해 **Trickle ICE** 방식을 권장한다. 이는 SDP를 먼저 교환한 후, 네트워크 후보(IP:Port)를 찾을 때마다 비동기적으로 상대방에게 보내는 방식이다.

하지만 WHIP은 HTTP 프로토콜을 기반으로 한다. HTTP는 기본적으로 '요청-응답'이 한 번 오고 가면 트랜잭션이 끝나는 구조다. Trickle ICE를 사용하려면 HTTP 응답 이후에도 추가 Candidate를 전송할 채널이 필요하다. WHIP 표준에는 PATCH 메서드로 Candidate를 추가하는 [명세](https://datatracker.ietf.org/doc/html/draft-ietf-wish-whip#section-4.3.1)가 있지만, 구현 복잡도를 고려해 우리는 **Full ICE** 전략을 선택한다.

Full ICE는 서버가 사용 가능한 모든 네트워크 후보(Candidate)를 찾을 때까지 기다렸다가, 이를 SDP Answer에 모두 포함시켜서 한 번의 HTTP 응답으로 연결 정보를 완성하는 방식이다.

### 3.3 Gathering Complete 대기

Pion 라이브러리는 `GatheringCompletePromise`라는 유용한 유틸리티를 제공한다. 이 함수는 Promise 패턴을 채택하여, Gathering이 완료되면 채널이 닫히며 블로킹이 해제된다.

우리는 `SetLocalDescription`을 호출하여 후보 수집을 시작한 뒤, 즉시 응답을 보내지 않고 수집이 완료될 때까지 고루틴을 블로킹한다.

```go
    // 13. ICE Candidate 수집 대기 준비
    // Pion의 Promise 패턴: Gathering이 완료되면 이 채널이 닫힌다.
    gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

    // 14. Local Description 설정 (Gathering 시작)
    // 이 시점부터 비동기적으로 ICE Candidate를 찾기 시작한다.
    if err := peerConnection.SetLocalDescription(answer); err != nil {
        http.Error(w, "Failed to set local description", http.StatusInternalServerError)
		log.Printf("SetLocalDescription failed: %v", err)
		peerConnection.Close()
        return
    }

    // 15. Gathering 완료 대기 (Blocking)
    <-gatherComplete
```

### 3.4 최종 응답 전송

대기가 끝나면 `peerConnection.LocalDescription()`에는 코덱 정보뿐만 아니라, 연결 가능한 서버의 IP와 포트 정보(`a=candidate`)까지 모두 포함되어 있다. 이제 이 완성된 SDP를 HTTP 201 응답과 함께 클라이언트에게 전송하면 핸드셰이크가 완료된다.

```go
    // 16. 최종 Answer 반환
    // Candidate가 포함된 최신 SDP를 가져온다.
    finalAnswer := peerConnection.LocalDescription()

    w.Header().Set("Content-Type", "application/sdp")
    w.Header().Set("Location", "/whip/session-uuid")
    w.WriteHeader(http.StatusCreated)
    fmt.Fprint(w, finalAnswer.SDP)

    log.Println("WHIP handler completed: Answer sent")
```

### 3.5 테스트: 연결 성립 확인

이제 서버는 완벽한 SDP Answer를 반환할 수 있다. 챕터 2에서 사용했던 `curl` 명령어를 다시 실행해 보자.

```bash
curl -i -X POST http://localhost:8080/whip \
  -H "Content-Type: application/sdp" \
  --data-binary "v=0
o=- 0 0 IN IP4 127.0.0.1
s=-
c=IN IP4 127.0.0.1
t=0 0
m=video 9 UDP/TLS/RTP/SAVPF 96
a=mid:0
a=sendonly
a=setup:actpass
a=ice-ufrag:testiceufrag
a=ice-pwd:testicepwd
a=rtpmap:96 VP8/90000
a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00
"
```

이전에는 "TODO" 텍스트만 왔다면, 이제는 서버가 보낸 거대한 SDP 문자열이 응답 바디에 출력될 것이다. 특히 내용을 자세히 보면 다음과 같은 `candidate` 라인들을 발견할 수 있다:

```text
a=candidate:842163049 1 udp 1234567890 x.x.x.x 50000 typ host
a=candidate:842163049 1 udp 1234567890 y.y.y.y 50000 typ srflx raddr 0.0.0.0 rport 50000
```

- **typ host:** 서버 로컬 주소 (Docker 내부에서는 컨테이너 IP)
- **typ srflx:** STUN 서버를 통해 확인한 공인 IP (Server Reflexive)

이 정보가 보인다면 클라이언트와 서버는 서로 합의된 코덱과 네트워크 경로를 모두 알게 되었다. 즉, 연결(Connection)은 성립되었다.

하지만 아직 한 가지가 빠졌다. 현재 코드에는 들어오는 영상을 처리할 핸들러(OnTrack)가 등록되어 있지 않다. Pion 라이브러리는 처리할 주인이 없는 트랙이 들어오면, SDP Answer 생성 시 해당 미디어 섹션을 자동으로 거절(Port == 0)해버린다. 즉, 연결은 성립되었으나, 실제 영상 데이터가 지나갈 통로는 서버가 막아버린 셈이다.

서버가 "영상 보내도 돼!"라고 문을 열어주려면(Port != 0), 트랙을 수신할 준비를 마쳐야 한다. 다음 챕터에서는 OnTrack 핸들러를 구현하여 닫힌 미디어 통로를 열고 실제 영상 데이터를 받아보자.