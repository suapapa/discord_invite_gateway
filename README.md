# Discord Invite Gateway

이 프로젝트는 디스코드 초대 링크가 자주 만료되는 문제를 해결하기 위해, 디스코드 봇 API를 이용하여 매번 검증된 초대 링크를 동적으로 생성하고, 설정한 비밀번호를 통해서만 해당 초대 링크에 접근할 수 있도록 보안 기능을 제공하는 미니멀 웹 서비스입니다.

---

## 1. 디스코드 봇 생성 및 설정 방법

서버가 디스코드 API를 호출하여 초대 링크를 생성할 수 있도록 디스코드 봇을 설정해야 합니다.

1. **디스코드 개발자 포털 접속**:
   - [Discord Developer Portal](https://discord.com/developers/applications)에 접속하여 로그인합니다.
2. **새 애플리케이션 생성**:
   - 우측 상단의 **New Application** 버튼을 클릭하고 이름을 지정합니다. (예: `Invite Gateway Bot`)
3. **봇(Bot) 활성화**:
   - 왼쪽 메뉴에서 **Bot** 탭을 선택하고 **Add Bot** 버튼을 클릭합니다.
   - **Token** 영역 아래의 **Reset Token** 버튼을 눌러 생성된 **봇 토큰(Token)**을 안전한 곳에 복사해 둡니다. 이 토큰이 `config.yaml`의 `discord.bot_token` 값이 됩니다.
4. **봇 서버 초대 링크 생성**:
   - 왼쪽 메뉴에서 **OAuth2** -> **URL Generator** 탭으로 이동합니다.
   - **Scopes**에서 `bot`을 선택합니다.
   - **Bot Permissions**에서 `Create Instant Invite` (초대 코드 만들기) 권한을 선택합니다.
   - 하단에 생성된 URL을 복사하여 웹 브라우저 주소창에 붙여넣고, 초대할 디스코드 서버를 선택하여 봇을 서버에 추가합니다.
5. **디스코드 대상 채널 ID 획득**:
   - 디스코드 앱 설정 -> 고급(Advanced) -> **개발자 모드(Developer Mode)**를 활성화합니다.
   - 초대 링크를 연결하고 싶은 서버의 텍스트 채널(예: `#welcome` 또는 `#일반`)을 우클릭하고 **ID 복사하기(Copy ID)**를 클릭합니다.
   - 이 값이 `config.yaml`의 `discord.channel_id` 값이 됩니다.

---

## 2. 프로젝트 설정

프로젝트 루트 디렉토리에 `config.yaml` 파일을 작성합니다. (`config.yaml`은 보안상 Git에 커밋되지 않습니다.)

```yaml
discord:
  bot_token: "YOUR_DISCORD_BOT_TOKEN" # 1단계에서 획득한 봇 토큰
  channel_id: "YOUR_DISCORD_CHANNEL_ID" # 초대 링크를 생성할 대상 채널 ID
web:
  password: "YOUR_SECRET_PASSWORD" # 웹페이지 조회를 위해 입력할 비밀번호
  port: 8080                    # 웹 서버가 작동할 포트 (기본값: 8080)
```

---

## 3. 실행 방법

### 로컬 개발 환경에서 실행

Go가 설치되어 있어야 합니다.

```bash
# 의존성 패키지 설치 및 실행
go run main.go
```

실행 후 웹 브라우저에서 `http://localhost:8080`으로 접속할 수 있습니다.

### 프로덕션 빌드 및 실행

```bash
# 바이너리 컴파일
go build -o discord_invite_gateway main.go

# 실행
./discord_invite_gateway
```

---

## 4. 웹페이지 사용 가이드

1. `http://localhost:8080` (혹은 배포된 도메인)에 접속하면 패스워드 입력창이 나타납니다.
2. `config.yaml`에 정의한 `web.password`를 입력하고 로그인합니다.
3. 인증에 성공하면, 현재 디스코드 서버의 최신 초대 정보(서버 이름 및 유효한 초대 링크)를 표시하는 간단한 카드 UI가 나타납니다.
4. "초대 링크 복사" 버튼을 눌러 손쉽게 복사하거나 "서버 접속하기" 버튼으로 바로 디스코드에 접속할 수 있습니다.
5. 브라우저를 닫으면 인증 세션이 자동으로 만료(Transient Session Cookie)되어 다시 접속할 때 패스워드 입력을 요구합니다.
