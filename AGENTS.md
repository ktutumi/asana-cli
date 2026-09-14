# AGENTS.md

このファイルは、このリポジトリ全体に適用する永続的な作業規約です。
ユーザーの明示的な指示と、対象ファイルに近い `AGENTS.md` を優先してください。

## Repository

- Go module: `github.com/ktutumi/asana-cli`
- Go toolchain: `go.mod` の Go 1.26
- Entry point: `cmd/asana-cli/main.go`
- CLI routing and rendering: `internal/cli`
- Resource commands and validation: `internal/cli/api_tasks.go`、`api_projects.go`、
  `api_misc.go`、`api_helpers.go`
- Asana and OAuth HTTP client: `internal/asana`
- Resource CRUD and uploads: `internal/asana/resources.go`、`attachments.go`
- OAuth URL and localhost callback: `internal/oauth`
- Credential persistence: `internal/config`
- User documentation: `README.md` and `README.ja.md`
- Bundled operator skill: `skills/asana-cli-operator/SKILL.md`（`--skill` で出力）

CLI は OAuth / `ASANA_PAT` 認証、resource の参照・作成・更新・削除、
関連付け、attachment upload、複製・template 化、job 確認に対応します。

Go、CLI、OAuth、Asana API の実装やレビューでは、該当する手順として
`.claude/skills/asana-cli-development/SKILL.md` を使用してください。

## Safety and Compatibility

- 実物の `client_secret`、`access_token`、`refresh_token`、`ASANA_PAT`、認可 `code`、credentials
  ファイルの内容を出力、ログ、スナップショット、コミットに含めない。
- ユーザーがその実行について明示的に依頼し、必要な認証情報を提供した場合を除き、
  実 Asana API を呼ばない。
- HTTP テストには `httptest.Server` と実行時 endpoint override を使い、設定を扱う
  テストには `t.TempDir()` または一時 `--config` パスを使う。
- CLI は標準ライブラリ中心の軽量な構成を保つ。大規模な再設計を依頼されない限り、
  CLI フレームワークを追加しない。
- 明示的な破壊的変更でない限り、既存のコマンド名、エイリアス、フラグを維持する。
- `README.md` と `README.ja.md` の内容を対応させ、それぞれ既存の言語と文体を保つ。

認証、OAuth、設定保存を変更する場合は、次の不変条件を維持してください。

- callback は `localhost` または `127.0.0.1` のみに bind する。
- OAuth `state` を生成し、callback で照合する。
- `auth login` の redirect URI は HTTP、path 必須、query / fragment なしとする。
- 空でない `ASANA_PAT` は API command と `auth status` で保存済み OAuth より優先し、
  config の読込・書込や自動 refresh を行わない。不正・拒否された PAT で OAuth へ fallback しない。
- `auth url` / `login` / `exchange` / `refresh` は PAT 設定時も明示的な OAuth 操作とする。
- `clientSecret` と PAT を永続保存しない。`auth status` は network request を行わない。
- ユーザー向け出力に `access_token` と `refresh_token` の実値を含めない。
- config directory は `0700`、credentials file は `0600` を維持する。
- 既定 config path は `XDG_CONFIG_HOME` を優先し、未設定時は `os.UserConfigDir()` を使う。

出力を変更する場合は、次の契約を維持してください。

- `json`: valid pretty-printed JSON
- `table`: collection は header row と tab-separated rows
- `table`: object は `field<TAB>value` lines
- `compact`: `field=value` lines
- `table` / `compact`: backslash、tab、CR、LF を escape して一行出力を保つ
- token 交換・login・refresh の出力は `json` 指定時以外は `compact`

## Change and Verification

- 振る舞いを変更する場合は、実用的な範囲で先に focused test を追加または更新する。
- CLI テストは buffer を設定した `CliIO` と
  `RunCLI(args, io, RuntimeOptions{})` を使う。
- command、flag、環境変数を変更した場合は、help と両方の README を更新する。
- 操作手順に影響する変更では同梱 operator skill も更新する。
- `--config` / `--output` は command より前に置く（`auth status --config` は例外）。
- API path の GID は escape し、list の offset pagination と search の非 pagination を区別する。
- 書込テストでは HTTP method・path・body と明示した field のみ送ることを検証する。
- PAT テストは `RuntimeOptions.PAT`、HTTP テストは `APIBase` / `TokenEndpoint` /
  `HTTPClient` を使い、環境変数のテストは `t.Setenv` で隔離する。
- 依頼範囲外の問題は勝手に修正せず、必要なら報告する。

リポジトリルートで、変更範囲に応じて次を実行してください。

```sh
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build -o /tmp/asana-cli ./cmd/asana-cli
```

資格情報を必要としない smoke check:

```sh
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli --skill
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
ASANA_PAT= go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```
