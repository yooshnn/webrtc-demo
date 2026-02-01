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

- Simple Request: `form-urlencoded`와 같은 오래된 방식으로, 하위 호환성을 위해 preflight를 사용하지 않는다. (대신 CSRF 토큰 등으로 보호해야 한다.)
- Complex Request: `application/json`이나 `application/sdp`와 같은 현대적인 방식이다.

Complex Request의 경우, 브라우저는 무턱대고 본 요청을 보내지 않는다. 대신 **Preflight** 라는 안전장치를 가동한다. 브라우저는 본 요청을 보내기 전, OPTIONS 메서드로 서버에게 먼저 묻는다.

> "`application/sdp` 타입으로 `POST` 보낼 건데, 받아줄 거야?"

이 질문에 대한 서버의 반응에 따라 브라우저의 행동이 갈린다.

- 허가: 서버가 `200 OK`와 함께 허가 헤더를 내려주면, 브라우저는 대기 중이던 본 요청(`POST`)을 전송한다.
- 거부: 서버가 거절하거나 응답하지 않으면, 브라우저는 본 요청을 전송하지 않는다.

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
    // text/plain 등의 Simple Request로 위장하여 preflight 우회 시도 차단
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

1. 201 Created: 방송 세션이라는 '리소스'가 서버에 새로 생성되었으므로, 일반적인 200 OK가 아닌 201 Created를 반환한다.
2. Location 헤더: 서버는 생성된 세션을 제어할 수 있는 고유 주소(Resource URL)를 알려줘야 한다. 클라이언트는 나중에 방송을 종료(DELETE)하거나 상태를 변경(PATCH)할 때 이 주소를 사용하게 된다.

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
# -d 옵션을 쓰면 curl은 기본적으로 application/x-www-form-urlencoded 로 보낸다.
curl -i -X POST http://localhost:8080/whip -d "v=0..."
```

#### 정상 동작 테스트

마지막으로, WHIP 표준에 맞게 헤더와 바디를 갖춰서 요청을 보내보자. 서버는 `201 Created` 응답과 함께, 방송을 제어할 수 있는 `Location` 헤더를 반환해야 한다.

```bash
curl -i -X POST http://localhost:8080/whip \
  -H "Content-Type: application/sdp" \
  -d "v=0..."
```

---

기본적인 HTTP 핸들러 구현이 완료되었다. 다음 단계로 **WebRTC PeerConnection**를 활용해, 실제 SDP 명함을 주고받는 과정을 구현해 보자.
