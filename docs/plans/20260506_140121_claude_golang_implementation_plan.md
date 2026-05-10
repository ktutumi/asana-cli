# Go 実装計画

## 目的

Rust 版 `asana-cli` の OAuth 認証と read-only Asana API CLI を、Go 標準ライブラリ中心の実装として構築する。

重視する互換性は次の 3 点です。

- コマンド互換
- 設定ファイル互換
- テスト互換

## 実装方針

- 実装は `test-driven-development` スキルを使用し、各振る舞いについて失敗するテストを先に書いて RED を確認してから本番コードを実装する。
- CLI framework は追加せず、標準ライブラリ中心で実装する。
- 実 Asana API には接続せず、`httptest.Server` と temp config path を使って検証する。
- `client_secret`, `access_token`, `refresh_token`, authorization `code` は出力・ログ・fixture に残さない。
- まず CLI の基盤とテスト可能性を固め、その後 OAuth / Asana API / command routing を実装する。

## タスク一覧

### Phase 1: Go プロジェクト基盤

#### 1. Go モジュールと最小エントリポイントを作る

Go プロジェクトの骨格を作り、CLI 実行口を `internal/cli` に委譲する。

**対象ファイル**

- `go.mod`
- `cmd/asana-cli/main.go`
- `internal/cli/cli.go`

**変更内容**

- module path は AGENTS の既定に従い、`github.com/ktutumi/asana-cli-go` に固定する。
- `main.go` は `os.Args[1:]` を次へ渡す。
  - `cli.RunCLI(args, cli.NewStdIO(), cli.NewRuntimeOptionsFromEnv())`
- `RunCLI` の戻り値を process exit code にする。

**Acceptance**

- `go test ./...` が通る。
- `go build -o /tmp/asana-cli ./cmd/asana-cli` が通る。
- `/tmp/asana-cli --help` が実 API へ接続せず終了する。

#### 2. CLI 入出力抽象とグローバルフラグ解析を TDD で実装する

サブコマンド実装前に、テスト可能な CLI 基盤を固める。

**対象ファイル**

- `internal/cli/cli.go`
- `internal/cli/cli_test.go`

**変更内容**

- 次の型・関数を作る。
  - `CliIO`
  - `RunCLI`
  - `RuntimeOptions`
  - `NewRuntimeOptionsFromEnv`
- サブコマンド前でグローバルフラグを解析する。
  - `--config`
  - `--output`
  - `--help` / `-h`
  - `--version` / `-V`
- 未知のグローバルフラグはエラーにする。
- 次の環境変数を override として扱う。
  - `ASANA_API_BASE`
  - `ASANA_OAUTH_TOKEN_ENDPOINT`
  - `BROWSER`

**Acceptance**

- help の単体テストが通る。
- version の単体テストが通る。
- 未知グローバルフラグの単体テストが通る。
- `--output=json|table|compact` の単体テストが通る。
- `--config=...` の単体テストが通る。

#### 3. サブコマンド用の軽量フラグパーサを実装する

`clap` の代替として、依存を増やさず厳密なフラグ検証を行う。

**対象ファイル**

- `internal/cli/cli.go`
- `internal/cli/cli_test.go`

**変更内容**

- 次の形式を扱う。
  - `--flag value`
  - `--flag=value`
  - boolean flag（`--help`, `--no-open`）
  - repeated `--scope`
  - positional target
- コマンドごとに許可フラグを限定する。
- 重複する positional target と named target を拒否する。
  - 例: `tasks get task-1 --task task-2`

**Acceptance**

- 未知サブコマンドフラグのテストが通る。
- 値必須フラグの値欠落テストが通る。
- inapplicable flag のテストが通る。
- extra positional のテストが通る。
- repeated scope のテストが通る。
- `ls` alias のテストが通る。

### Phase 2: 設定と OAuth 基盤

#### 4. 設定ファイル互換を実装する

Rust 版の `credentials.json` をそのまま読み書きできるようにする。

**対象ファイル**

- `internal/config/config.go`
- `internal/config/config_test.go`

**変更内容**

- 次の構造体を定義する。
  - `StoredConfig{clientId, redirectUri, token}`
  - `TokenData{access_token, refresh_token, token_type, expires_in, expires_at}`
- `LoadConfig` は、ファイルが存在しない場合に空設定を返す。
- `SaveConfig` は既存値と merge する。
- `clientSecret` は保存しない。
- 設定ディレクトリは private にする。
- Unix では設定ファイルの permission を `0600` に維持する。
- 既定パスは次のいずれかにする。
  - `$XDG_CONFIG_HOME/asana-cli/credentials.json` 相当
  - `os.UserConfigDir()/asana-cli/credentials.json`

**Acceptance**

- temp dir で load / save / merge のテストが通る。
- `0600` のテストが通る。
- JSON key 互換のテストが通る。
- 未存在 config のテストが通る。
- テスト・ログに実 token を出さない。

#### 5. OAuth URL と state 生成を実装する

認可 URL の文字列互換を固定する。

**対象ファイル**

- `internal/oauth/oauth.go`
- `internal/oauth/oauth_test.go`

**変更内容**

- 認可 endpoint を定義する。
  - `https://app.asana.com/-/oauth_authorize`
- default scopes を定義する。
  - `users:read`
  - `workspaces:read`
  - `projects:read`
  - `tasks:read`
  - `stories:read`
  - `attachments:read`
- default localhost redirect を定義する。
  - `http://127.0.0.1:18787/callback`
- state は 32 byte random を URL-safe base64 no padding で生成する。
- query の space は `+` ではなく `%20` にする。

**Acceptance**

- URL 完全一致のテストが通る。
- state 長 43 / 文字種のテストが通る。
- default scopes のテストが通る。
- default redirect URI のテストが通る。

#### 6. localhost callback server を実装する

`auth login` の安全な localhost redirect 受信を作る。

**対象ファイル**

- `internal/oauth/callback.go`
- `internal/oauth/callback_test.go`

**変更内容**

- `net.Listen` + `http.Server` で指定 host / port / path を listen する。
- port `0` をテスト用に許可し、実際の callback URL を返す。
- path 不一致は 404 にする。
- `code` 欠落は 400 と error にする。
- 成功時は code / state を返す。
- timeout / cancel で clean shutdown する。

**Acceptance**

- callback 成功のテストが通る。
- code 欠落のテストが通る。
- favicon など余分な request のテストが通る。
- timeout のテストが通る。
- port `0` のテストが `httptest` / localhost のみで通る。

### Phase 3: Asana HTTP client

#### 7. Asana HTTP client の token 処理を実装する

OAuth token endpoint 呼び出しを hermetic test で固定する。

**対象ファイル**

- `internal/asana/client.go`
- `internal/asana/client_test.go`

**変更内容**

- 次の関数を実装する。
  - `NewClient(apiBase, tokenEndpoint)`
  - `ExchangeCodeForToken`
  - `RefreshAccessToken`
- `application/x-www-form-urlencoded` で送信する。
- `expires_in` があり `expires_at` が無い場合は、UTC RFC3339 で補完する。
- テストしやすいように、必要なら clock injection を最小限追加する。

**Acceptance**

`httptest.Server` で次のテストが通る。

- `grant_type`
- request body
- request header
- `expires_at` 補完
- refresh token

#### 8. Asana read-only API client を実装する

Rust 版の endpoint と pagination を Go に移植する。

**対象ファイル**

- `internal/asana/client.go`
- `internal/asana/client_test.go`

**変更内容**

- 次の method を追加する。
  - `FetchMe`
  - `ListWorkspaces`
  - `ListProjects`
  - `ListTasks`
  - `GetTask`
  - `ListSubtasks`
  - `ListStories`
  - `ListComments`
  - `ListAttachments`
- `next_page.offset` がある限り、`offset` query を差し替えて全ページ取得する。
- `ListComments` は `/tasks/{gid}/stories` を使う。
- `ListComments` には次の `opt_fields` を付ける。
  - `gid`
  - `resource_subtype`
  - `resource_type`
  - `text`
  - `html_text`
  - `created_at`
  - `created_by.name`
- `ListComments` は `resource_subtype == comment_added` のみ返す。
- path segment は `url.PathEscape` などで特殊文字を安全に扱う。

**Acceptance**

- Authorization header のテストが通る。
- 各 endpoint のテストが通る。
- pagination のテストが通る。
- 特殊文字 gid の path escape テストが通る。
- comments filter のテストが通る。
- Asana error envelope（`errors[].message` join）のテストが通る。

### Phase 4: CLI 出力とコマンド実装

#### 9. 出力 renderer を実装する

`json` / `table` / `compact` の互換仕様を CLI 層で固定する。

**対象ファイル**

- `internal/cli/cli.go`
- `internal/cli/cli_test.go`

**変更内容**

- JSON は `json.MarshalIndent` の pretty JSON にする。
- collection の table は header 付き TSV にする。
- collection の compact は header なしで 1 item 1 line とし、各 line 内の configured fields を `field=value` ペアで出す。
- object の table は `field\tvalue` にする。
- object compact は 1 field 1 line の `field=value` にする。
- `compact` は collection / object とも常に `field=value` 形式を使う。
- 次の対象ごとに列定義を用意する。
  - workspaces
  - projects
  - tasks
  - task details
  - subtasks
  - stories
  - comments
  - attachments
  - me
- `created_by.name` のような dot path を解決する。
- 値内の `\\`, tab, CR, LF はエスケープする。

**Acceptance**

- workspace JSON / table / compact のテストが通る。
- workspace compact は `gid=...` / `name=...` など configured fields の `field=value` ペアを含む 1 item 1 line 形式である。
- comments の selected columns テストが通る。
- attachments columns のテストが通る。
- 改行 / tab / backslash escape のテストが通る。
- nested field のテストが通る。

#### 10. auth サブコマンドを実装する

認証フローを、既存ユーザーの設定互換と secret redaction を守って実装する。

**対象ファイル**

- `internal/cli/cli.go`
- `internal/cli/cli_test.go`

**変更内容**

- 次の command を実装する。
  - `auth url`
  - `auth exchange`
  - `auth login`
  - `auth refresh`
  - `auth status`
- `auth login` は OOB を拒否する。
- `auth login` は次の条件をすべて満たす redirect URI のみ許可する。
  - scheme は `http`
  - host は `127.0.0.1` または `localhost`
  - query / fragment を含まない
- `auth exchange` / `auth refresh` の flag contract は次に固定する。
  - `auth exchange`
    - `--code`: 必須 flag
    - `--client-id`: flag 優先。未指定時は config fallback 可
    - `--redirect-uri`: flag 優先。未指定時は config / default fallback 可
    - `--client-secret`: 必須 flag
    - `--refresh-token`: 不許可
  - `auth refresh`
    - `--refresh-token`: flag 優先。未指定時は config fallback 可
    - `--client-id`: flag 優先。未指定時は config fallback 可
    - `--client-secret`: 必須 flag
    - `--code` / `--redirect-uri`: 不許可
  - `client_secret` は token endpoint の form body には含めるが、設定ファイルには保存しない。
- 次の flag を扱う。
  - `--no-open`
  - `--listen-timeout-ms`
  - `--scope`
  - `--state`
- browser open は `$BROWSER` / OS default を試みる。
- browser open に失敗した場合は、手動で URL を開く案内を stderr に出す。
- token 表示では `access_token` / `refresh_token` を `***` に redaction する。

**Acceptance**

- `auth url` のテストが通る。
- exchange 保存のテストが通る。
- `auth exchange` は `--code` / `--client-secret` missing を拒否するテストが通る。
- `auth exchange` は `--client-id` / `--redirect-uri` の config fallback を使うテストが通る。
- `auth exchange` は inapplicable flag `--refresh-token` を拒否するテストが通る。
- status missing / present redaction のテストが通る。
- refresh merge のテストが通る。
- `auth refresh` は `--client-secret` missing を拒否するテストが通る。
- `auth refresh` は `--refresh-token` / `--client-id` の config fallback を使うテストが通る。
- `auth refresh` は inapplicable flag `--code` / `--redirect-uri` を拒否するテストが通る。
- token endpoint request の form body に `client_secret` が含まれ、保存後 config に `client_secret` が含まれないテストが通る。
- login callback 成功のテストが通る。
- state mismatch のテストが通る。
- browser open fallback のテストが通る。
- `auth login` は `http://127.0.0.1/callback?x=1` を拒否するテストが通る。
- `auth login` は `http://localhost/callback#frag` を拒否するテストが通る。
- `auth login` は `localhost` / `127.0.0.1` 以外の host を拒否するテストが通る。
- help が必須フラグ validation を走らせないテストが通る。

#### 11. read-only CLI サブコマンドを実装する

Rust 版の公開コマンドと alias を揃える。

**対象ファイル**

- `internal/cli/cli.go`
- `internal/cli/cli_test.go`

**変更内容**

- 次の command を route する。
  - `me`
  - `workspaces list|ls`
  - `projects list|ls`
  - `project list|ls` alias
  - `tasks list|ls|get|subtasks|stories|comments|attachments`
- `projects list <workspace_gid>` と `--workspace` をサポートする。
- `tasks ... <task_gid>` と `--task` をサポートする。
- `tasks list <project_gid>` と `--project` をサポートする。
- access token 未保存時は、`auth login` と manual flow を案内する。

**Acceptance**

- saved access token が Authorization header に使われるテストが通る。
- `projects` / `project` alias のテストが通る。
- task positional / named target のテストが通る。
- missing token error のテストが通る。
- 各 command help のテストが通る。

### Phase 5: ドキュメントと workflow

#### 12. ヘルプ文言と README を Go 版へ更新する

ユーザー向け仕様を日本語トーンで維持する。

**対象ファイル**

- `README.md`（存在しなければ新規作成）
- `README.ja.md`（存在させる場合。存在しなければ新規作成）
- `internal/cli/cli.go`

**変更内容**

- インストール手順を Go に変更する。
  - `go install github.com/ktutumi/asana-cli-go/cmd/asana-cli@latest`
  - `go build`
- README / `go install` 例は module path `github.com/ktutumi/asana-cli-go` を前提にする。
- 次の内容を Go 版に合わせる。
  - コマンド一覧
  - OAuth app setup
  - 出力形式
  - config file
  - 環境変数
  - セキュリティ方針
  - manual flow
  - release asset 名
- help は primary commands を表示する。
- legacy `project` alias は README / 詳細に留めるか、root help では目立たせない。

**Acceptance**

- README の command list が CLI 実装と一致する。
- `go run ./cmd/asana-cli --help` の文言テストが通る。

#### 13. CI を Go に置き換える

Rust 固有 workflow を Go の検証へ移行する。

**対象ファイル**

- `.github/workflows/ci.yml`（存在しなければ `.github/workflows/` ごと新規作成）

**変更内容**

- `actions/setup-go` を使う。
- 次を実行する。
  - `go test ./...`
  - `go vet ./...`
  - `gofmt` check
- live Asana API へ接続しない。

**Acceptance**

- pull request / push で Go tests のみを実行する設定になっている。

#### 14. Release workflow を Go cross build に置き換える

既存 release asset 互換を可能な範囲で保つ。

**対象ファイル**

- `.github/workflows/release.yml`（存在しなければ `.github/workflows/` ごと新規作成）

**変更内容**

- tag push で次の target を `go build` する。
  - `linux/amd64`
  - `darwin/amd64`
  - `darwin/arm64`
- `asana-cli-${VERSION}-${target}.tar.gz` と `.sha256` を作る。
- 必要なら `CGO_ENABLED=0` を設定する。

**Acceptance**

- workflow dry review で Rust / Cargo 参照が残らない。
- 生成 asset 名が README と一致する。

### Phase 6: 最終検証

#### 15. 最終検証と secret / security review を行う

変更全体が安全で互換性を満たすことを確認する。

**対象ファイル**

- `internal/**`
- `cmd/asana-cli/main.go`
- `README.md`

**変更内容**

- gofmt を実行する。
- tests を実行する。
- build を実行する。
- smoke checks を実行する。
- 実 API は呼ばない。
- 手動確認は次のみにする。
  - `auth url --client-id dummy --state fixed`
  - `auth status --config $(mktemp -d)/credentials.json`
  - `--help`
  - `--version`

**Acceptance**

- `gofmt` 差分なし。
- `go test ./...` 成功。
- `go build -o /tmp/asana-cli ./cmd/asana-cli` 成功。
- secret が stdout / stderr / test fixture / README に漏れていない。

## 変更対象ファイル

既存ファイルは更新する。README / workflow は、存在しなければ新規作成する。

- `README.md`（既存なら更新、なければ新規作成）  
  Rust の開発・インストール・release 説明を Go 実装に合わせて更新し、`go install github.com/ktutumi/asana-cli-go/cmd/asana-cli@latest` を前提にする。
- `README.ja.md`（既存なら更新、なければ必要に応じて新規作成）  
  README と同じコマンド仕様・Go 手順へ更新する。
- `.github/workflows/ci.yml`（既存なら更新、なければ `.github/workflows/` ごと新規作成）  
  Cargo CI から Go CI へ置換する。
- `.github/workflows/release.yml`（既存なら更新、なければ `.github/workflows/` ごと新規作成）  
  Cargo release から Go cross build release へ置換する。
- `tasks/todo.md`  
  実装担当が作業開始時にチェックリストを記録する場合のみ更新する。

## 新規作成ファイル

- `go.mod`  
  Go module 定義。module path は `github.com/ktutumi/asana-cli-go` に固定する。
- `cmd/asana-cli/main.go`  
  CLI binary entrypoint。
- `internal/cli/cli.go`  
  CLI parsing, routing, rendering, auth / config orchestration。
- `internal/cli/cli_test.go`  
  CLI behavior, output, auth flow integration tests。
- `internal/asana/client.go`  
  Asana API / OAuth HTTP client。
- `internal/asana/client_test.go`  
  HTTP client unit tests using `httptest.Server`。
- `internal/config/config.go`  
  credentials load / save / merge / default path。
- `internal/config/config_test.go`  
  config JSON / permission tests with `t.TempDir()`。
- `internal/oauth/oauth.go`  
  authorization URL, default scopes, state generation, callback parsing helpers。
- `internal/oauth/oauth_test.go`  
  OAuth URL / state tests。
- `internal/oauth/callback.go`  
  localhost callback HTTP server。
- `internal/oauth/callback_test.go`  
  callback server tests。

## 依存関係

- Task 1 は、すべての Go package 実装タスクの前に完了する。
- Tasks 2 and 3 は、Tasks 10 and 11 の command-specific work より前に完了する。
- Task 4 は、Task 10 の auth persistence と Task 11 の access-token loading の前提になる。
- Tasks 5 and 6 は、Task 10 の `auth url` / `auth login` の前提になる。
- Tasks 7 and 8 は、Task 11 の CLI API commands の前提になる。
- Task 9 は Task 2 に依存し、Tasks 10 and 11 の CLI output assertions の前提になる。
- Tasks 12 to 14 は、command behavior が安定した後に行う。
- Task 15 は、すべての implementation / documentation / workflow tasks に依存する。

## リスクと確認事項

### 1. 参照 context が存在しなかった

指定された `/var/.../context.md` は読み取り時点で存在しなかった。

そのため、この計画はプロンプト内の Previous step output とローカル Rust / Go 参照リポジトリを根拠にしている。

### 2. Go version の決定が必要

Go version は実行環境 / 配布方針に合わせて決める必要がある。

既存参照では `go 1.26.2` が見えるが、CI で利用可能な version と整合させること。

### 3. `auth login` の localhost 制限は security-critical

redirect URI は `http` scheme、host が `localhost` / `127.0.0.1`、query / fragment なしの場合のみ許可する。
それ以外の host、query 付き URI、fragment 付き URI を誤って許可しないよう、テストで固定する。

### 4. secret を漏らさない

次の値は実値を出力・ログ・fixture に残さない。

- `client_secret`
- `access_token`
- `refresh_token`
- authorization `code`

また、token stdout は必ず redaction する。

### 5. `compact` 出力仕様の説明に注意する

`compact` は AGENTS / skill の `field=value` contract に従う。
collection は 1 item 1 line の `field=value` ペア、object は 1 field 1 line の `field=value` として README / ヘルプ / テスト期待値を揃える。

### 6. comments は stories endpoint + filter

comments は専用 endpoint ではなく、stories endpoint と `comment_added` filter で実装する。

漏れやすい移植ポイントなので、Asana client と CLI の両方でテストする。

### 7. Release workflow の asset 名確認が必要

Release workflow の archive 名を次のどちらにするか、配布方針確認が必要。

- 既存 Rust release と完全互換にする
- Go module / repo 名を反映して変える
