# octunnel

[OpenCode](https://opencode.ai)를 로컬에서 실행한 뒤, 외부에서도 접속할 수 있게 해주는 오픈소스 CLI 도구입니다. [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/)을 통해 로컬 서버를 HTTPS 주소로 공개합니다 — 명령어 하나로.

> [!CAUTION]
> **터널 URL을 통해 OpenCode 서버가 외부에 공개됩니다.**
> 링크를 가진 누구나 서버에 접근할 수 있으므로, URL을 공개적으로 공유하지 마세요.
> Quick Tunnel은 매 실행마다 임의의 일회성 URL을 생성하지만, 해당 URL을 아는 사람은 접근할 수 있습니다.
> Named Tunnel을 사용하는 경우, [Cloudflare Access](https://developers.cloudflare.com/cloudflare-one/policies/access/)를 통한 제로트러스트 인증 적용을 권장합니다.

## 설치

```bash
# 빠른 설치 (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/chabinhwang/octunnel/main/install.sh | bash

# Homebrew
brew install chabinhwang/tap/octunnel

# Go
go install github.com/chabinhwang/octunnel@latest
```

### 플랫폼 지원

| 플랫폼 | 상태 |
|--------|------|
| macOS | 완전 지원 |
| Linux | 완전 지원 |
| Windows | 미지원 (Unix 시스템콜 필요) |

### 사전 요구사항

<details>
<summary><strong>macOS</strong></summary>

```bash
npm install -g opencode
brew install cloudflared
# lsof는 기본 내장
```
</details>

<details>
<summary><strong>Linux (Debian/Ubuntu)</strong></summary>

```bash
npm install -g opencode

# cloudflared
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg \
  | sudo tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared $(lsb_release -cs) main" \
  | sudo tee /etc/apt/sources.list.d/cloudflared.list
sudo apt update && sudo apt install cloudflared

# 클립보드 (URL 자동 복사용, 선택사항)
sudo apt install xclip   # 또는 xsel

# lsof는 대부분의 배포판에 기본 내장
```
</details>

<details>
<summary><strong>Linux (Arch)</strong></summary>

```bash
npm install -g opencode
pacman -S cloudflared xclip
# lsof는 기본 내장
```
</details>

<details>
<summary><strong>Windows</strong> (미지원)</summary>

현재 버전은 Unix 전용 프로세스 관리(`Setpgid`, `lsof`, POSIX 시그널)를 사용하므로 Windows에서는 동작하지 않습니다.

향후 지원을 위해 사전 설치만 해두려면:

```powershell
npm install -g opencode
winget install Cloudflare.cloudflared   # 또는: choco install cloudflared
# 클립보드: Windows 내장 'clip' 명령어
# 포트 감지: netstat -ano | findstr LISTENING
```
</details>

## 업데이트

```bash
# Homebrew
brew update && brew upgrade octunnel

# curl로 설치한 경우 (재실행하면 최신 버전으로 덮어씀)
curl -fsSL https://raw.githubusercontent.com/chabinhwang/octunnel/main/install.sh | bash

# Go
go install github.com/chabinhwang/octunnel@latest
```

## 빠른 시작

### 한 줄로 공개 URL 생성 (Quick Tunnel)

```bash
octunnel
```

실행하면 다음이 자동으로 진행됩니다:

1. `opencode serve` 실행
2. 로컬 포트 감지
3. Cloudflare Quick Tunnel 생성 (`*.trycloudflare.com`)
4. 공개 URL 클립보드 복사
5. 터미널에 QR 코드 출력

로그인이나 설정 없이 바로 사용할 수 있습니다.

### 고정 도메인으로 공개 (Named Tunnel)

각 명령어는 완료 후 **다음 단계**를 안내합니다:

```bash
# 1단계: Cloudflare 로그인 (브라우저가 열림)
octunnel login

# 2단계: 터널 생성 + DNS 연결
octunnel auth

# 3단계: 고정 도메인으로 실행
octunnel run
```

### 도메인 변경

```bash
octunnel switch domain
```

다른 Cloudflare 도메인으로 재로그인하고 DNS 라우팅을 변경합니다.

## 명령어 상세

### `octunnel`

가장 간단한 실행 방법입니다. 로그인 없이 Quick Tunnel을 열어 임시 공개 URL을 생성합니다.

- `opencode serve` 자동 실행
- stdout 파싱 + `lsof` 기반 포트 감지
- `cloudflared tunnel --url` 으로 Quick Tunnel 생성
- 공개 URL 클립보드 복사 + QR 코드 출력

### `octunnel login`

Cloudflare Tunnel 로그인 및 기본 도메인 등록.

- `cloudflared tunnel login` 실행 (브라우저 인증)
- `cert.pem` 경로를 자동 파싱하여 저장
- 기본 도메인 입력 및 확인

**보수적 복구 정책**: 로그인 상태가 조금이라도 불명확하면 기존 `cert.pem`을 삭제하고 처음부터 다시 로그인합니다. 기존 인증서를 재사용하지 않습니다.

### `octunnel auth`

Named Tunnel 생성 및 DNS 연결.

- `cloudflared tunnel create octunnel` 으로 터널 생성
- 터널명 중복 시 `octunnel1`, `octunnel2`, ... 순서로 재시도
- 서브도메인 입력 → `cloudflared tunnel route dns` 로 DNS 연결
- 기존 CNAME 덮어쓰기 경고 후 확인

### `octunnel run`

설정된 Named Tunnel을 실행합니다.

- `opencode serve` 실행 + 포트 감지
- `~/.octunnel/cloudflared.yml` 자동 갱신 (매 실행마다 포트 재감지)
- `cloudflared tunnel run` 실행
- 고정 URL `https://<hostname>` 출력

### `octunnel switch domain`

기존 터널은 유지하면서 도메인만 변경합니다.

- 기존 `cert.pem` 백업
- 새 도메인으로 재로그인
- 새 DNS 라우팅
- 모든 단계 성공 후에만 이전 백업 삭제

### `octunnel reset`

설정을 초기화하고 Cloudflare 터널을 삭제합니다.

- 모든 octunnel 상태 및 로컬 설정 파일 삭제
- CNAME DNS 레코드는 **자동 삭제되지 않습니다** — Cloudflare 대시보드에서 수동 제거 필요

### `octunnel remove`

octunnel 데이터를 완전히 제거합니다.

- `~/.octunnel` 디렉토리 전체 삭제
- octunnel 바이너리 자체는 삭제하지 않음 (필요 시 수동 삭제)
- 기존 Cloudflare 리소스(터널, CNAME)는 삭제하지 않음

## 상태 파일

모든 상태는 `~/.octunnel/config.json`에 저장됩니다.

```json
{
  "certPemPath": "/Users/me/.cloudflared/cert.pem",
  "baseDomain": "example.com",
  "tunnelId": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "tunnelName": "octunnel",
  "credentialsFilePath": "/Users/me/.cloudflared/xxxxxxxx.json",
  "hostname": "open.example.com",
  "operationStatus": "completed",
  "currentPhase": "hostname_saved",
  "mode": "named"
}
```

- 원자적 쓰기: `config.json.tmp`에 먼저 쓴 뒤 `rename`
- 매 저장 시 `config.json.bak` 백업 생성
- 파싱 실패 시 `.bak`에서 자동 복구

## `~/.octunnel/cloudflared.yml` 관리

octunnel은 `~/.cloudflared/config.yml`을 **절대 수정하지 않습니다**. 대신 `~/.octunnel/cloudflared.yml`에 별도 설정 파일을 생성하고, `cloudflared tunnel run` 실행 시 `--config` 플래그로 전달합니다.

- 원자적 쓰기: `.tmp`에 먼저 쓴 뒤 `rename`으로 교체 (부분 쓰기 방지)

## 복구 동작

octunnel은 중간 실패, Ctrl+C, 프로세스 비정상 종료 등에 대비한 복구 로직을 내장하고 있습니다.

### 프로세스 복구

| 상황 | 동작 |
|------|------|
| opencode + cloudflared 둘 다 살아있음 | 프로세스명 검증(PID만이 아닌) 후 기존 세션 재사용, URL 재출력 |
| opencode만 살아있음 | 포트 재감지 후 cloudflared만 재시작 |
| cloudflared만 살아있음 | orphan으로 간주, 종료 후 전체 재시작 |
| 둘 다 죽음 | 처음부터 재시작 |

### 명령별 복구

| 상황 | 동작 |
|------|------|
| `login` 중 중단 | cert.pem 삭제 + 재로그인 강제 |
| `auth`에서 터널 생성 후 DNS 전에 중단 | 터널 생성 건너뛰고 DNS부터 재개 |
| `run`에서 cloudflared.yml 쓰기 실패 | 원자적 쓰기(tmp + rename)로 부분 쓰기 방지 |
| `switch domain` 중 새 로그인 실패 | 이전 cert.pem 백업에서 복원 |
| config.json 손상 | `.bak` 파일에서 자동 복구 |

### 중복 방지

- Lock 파일(`~/.octunnel/octunnel.lock`)로 동시 실행 차단
- 죽은 PID의 stale lock은 자동 정리; PID가 살아있으면(다른 프로세스라도) 수동 삭제 필요
- 이미 생성된 터널은 다시 생성하지 않음
- DNS route가 이미 존재하면 성공으로 간주

## 로그 태그

| 태그 | 의미 |
|------|------|
| `[preflight]` | 의존성 검사 |
| `[opencode]` | opencode serve 관련 |
| `[cloudflared]` | cloudflared 관련 |
| `[octunnel]` | octunnel 자체 동작 |
| `[recover]` | 복구 동작 |
| `[warn]` | 경고 |
| `[error]` | 에러 |

## 삭제

```bash
# 1. 설정, 터널, octunnel 데이터 전부 삭제
octunnel remove

# 2. 바이너리 삭제
brew uninstall octunnel          # Homebrew로 설치한 경우
# 또는
rm $(which octunnel)             # curl이나 go install로 설치한 경우
```

> **참고:** `octunnel remove`는 Cloudflare 터널은 삭제하지만, CNAME DNS 레코드는 자동 삭제하지 않습니다. [Cloudflare 대시보드](https://dash.cloudflare.com)에서 직접 삭제해주세요.

## 알려진 한계

- **Windows 미지원** — Unix 전용 시스템콜(`Setpgid`, `lsof`, POSIX 시그널) 필요
- Quick Tunnel URL은 매 실행마다 변경됩니다
- `cloudflared tunnel login`은 브라우저가 필요합니다 (headless 환경에서는 수동 cert 설정 필요)
- 포트 감지는 `lsof` 출력 파싱에 의존합니다 (macOS/Linux)
- 클립보드: `pbcopy`(macOS), `xclip`/`xsel`(Linux) — Windows `clip`은 미연동
- 프로세스 감지에 `ps`와 `pgrep`를 사용합니다 (macOS/Linux에 기본 설치됨)
- Cloudflare API를 사용하지 않습니다 — 모든 작업은 `cloudflared` CLI로 처리

## 라이선스

MIT
