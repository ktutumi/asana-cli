# AGENTS.md

このファイルは、このリポジトリ全体に適用する永続的な作業規約です。
ユーザーの明示的な指示と、対象ファイルに近い `AGENTS.md` を優先してください。

## Repository

- Go module: `github.com/ktutumi/asana-cli-go`
- Entry point: `cmd/asana-cli/main.go`
- CLI routing and rendering: `internal/cli`
- Asana and OAuth HTTP client: `internal/asana`
- OAuth URL and localhost callback: `internal/oauth`
- Credential persistence: `internal/config`
- User documentation: `README.md` and `README.ja.md`

Go、CLI、OAuth、Asana API の実装やレビューでは、該当する手順として
`.claude/skills/asana-cli-go-development/SKILL.md` を使用してください。

## Safety and Compatibility

- 実物の `client_secret`、`access_token`、`refresh_token`、認可 `code`、credentials
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
- ユーザー向け出力に `access_token` と `refresh_token` の実値を含めない。
- config directory は `0700`、credentials file は `0600` を維持する。

出力を変更する場合は、次の契約を維持してください。

- `json`: valid pretty-printed JSON
- `table`: collection は header row と tab-separated rows
- `compact`: `field=value` lines
- `table` / `compact`: backslash、tab、CR、LF を escape して一行出力を保つ

## Change and Verification

- 振る舞いを変更する場合は、実用的な範囲で先に focused test を追加または更新する。
- CLI テストは buffer を設定した `CliIO` と
  `RunCLI(args, io, RuntimeOptions{})` を使う。
- command、flag、環境変数を変更した場合は、help と両方の README を更新する。
- 依頼範囲外の問題は勝手に修正せず、必要なら報告する。

リポジトリルートで、変更範囲に応じて次を実行してください。

```sh
gofmt -w <changed-go-files>
go test ./...
go build -o /tmp/asana-cli ./cmd/asana-cli
```

資格情報を必要としない smoke check:

```sh
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```
