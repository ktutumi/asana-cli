# asana-cli

言語: [English](README.md) | 日本語

Go で書いた個人利用向け Asana OAuth / API CLI です。GitHub Releases から macOS / Linux 向けバイナリを配布できる前提で構成しています。

主な機能:
- `auth url` で認可 URL を生成
- `auth exchange` で authorization code を token に交換
- `auth login` で localhost callback による自動ログイン
- `auth status` で保存済み認証情報の状態を確認
- `auth refresh` で refresh token を使って access token を更新
- `me`
- `workspaces list`
- task、subtask、project、section、comment、membership、関連付けの読み書き
- task 検索と custom ID による取得
- file または外部 URL の attachment 登録
- resource の複製・削除、project template 化、非同期 job の確認

セキュリティ/UX 方針:
- 設定ファイルは XDG Base Directory (`$XDG_CONFIG_HOME/asana-cli/credentials.json`) を優先
- 設定ファイル権限は `0600` を維持
- `clientSecret` は保存しない
- 標準出力に token を出すときは `access_token` / `refresh_token` を redact
- `auth login` は `http://127.0.0.1/...` または `http://localhost/...` の redirect URI のみ許可

## インストール

### go install

```bash
go install github.com/ktutumi/asana-cli-go/cmd/asana-cli@latest
```

### ソースからビルド

```bash
go build -o /tmp/asana-cli ./cmd/asana-cli
/tmp/asana-cli --help
```

### リリースバイナリ

GitHub Releases から以下を配布します。
- `linux-amd64`
- `darwin-amd64`
- `darwin-arm64`

Releases 一覧:
- https://github.com/ktutumi/asana-cli-go/releases

各 archive には対応する `.sha256` ファイルも添付されます。

ファイル名の例:
- `asana-cli-vX.Y.Z-linux-amd64.tar.gz`
- `asana-cli-vX.Y.Z-linux-amd64.tar.gz.sha256`

ダウンロード例:

Linux amd64:
```bash
VERSION=v0.1.0
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-linux-amd64.tar.gz
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-linux-amd64.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-linux-amd64.tar.gz.sha256
```

macOS Intel:
```bash
VERSION=v0.1.0
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-darwin-amd64.tar.gz
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-darwin-amd64.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-darwin-amd64.tar.gz.sha256
```

macOS Apple Silicon:
```bash
VERSION=v0.1.0
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-darwin-arm64.tar.gz
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-darwin-arm64.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-darwin-arm64.tar.gz.sha256
```

macOS で "Apple はマルウェアが含まれていないことを検証できませんでした" と表示される場合:
```bash
xattr -dr com.apple.quarantine ./asana-cli
./asana-cli --help
```

別の回避方法:
- Finder で `asana-cli` を右クリックして「開く」
- もしくは「システム設定 → プライバシーとセキュリティ」から `このまま開く`

補足:
- 現在の配布バイナリは notarization されていないため、macOS では Gatekeeper による確認ダイアログが出ることがあります。
- 上記の `xattr` 解除は、ダウンロード済みバイナリをローカルで使うための回避策です。

展開例:
```bash
VERSION=v0.1.0
tar -xzf asana-cli-${VERSION}-linux-amd64.tar.gz
./asana-cli --help
```

## Asana OAuth アプリ設定

Asana Developer Console で OAuth アプリを作成し、redirect URI を正確に登録してください。

例:
- `urn:ietf:wg:oauth:2.0:oob`
- `http://127.0.0.1:18787/callback`

注意:
- `auth login` は localhost callback 専用です
- OOB/manual copy-paste を使うときは `auth url` + `auth exchange` を使ってください
- localhost callback で `:0` はテスト用です。本番運用では固定ポートを登録してください

## 使い方

### 出力形式を選ぶ

既定の出力形式は `table` です。必要に応じて `--output json` または `--output compact` を指定します。

```bash
asana-cli --output json workspaces list
asana-cli --output table workspaces list
asana-cli --output compact tasks comments 789
```

使い分け:
- `json`: pretty JSON。`jq` などで処理しやすい
- `table`: ヘッダ付きの TSV 風表示。人が一覧を眺めやすい
- `compact`: `field=value` の簡潔表示。collection は 1 item 1 line で表示

### 認可 URL を出す

```bash
asana-cli auth url \
  --client-id "$ASANA_CLIENT_ID" \
  --state demo-state
```

### manual flow で code を交換する

```bash
asana-cli auth exchange \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri urn:ietf:wg:oauth:2.0:oob \
  --code "$ASANA_CODE"
```

### localhost callback で自動ログインする

```bash
asana-cli auth login \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri http://127.0.0.1:18787/callback
```

ブラウザを自動起動したくない場合:

```bash
asana-cli auth login \
  --no-open \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri http://127.0.0.1:18787/callback
```

期待される挙動:
1. CLI がブラウザで開くべき URL を出力
2. 可能ならブラウザを自動起動し、失敗時は URL を手動で開くよう案内
3. localhost callback が `code` と `state` を受信
4. token を交換して設定ファイルへ保存

### 保存済み認証情報の状態を確認する

```bash
asana-cli auth status
```

表示内容:
- `clientId` / `redirectUri`
- access token / refresh token の有無（値そのものは redact）
- `expires_at`

### token を refresh する

```bash
asana-cli auth refresh --client-secret "$ASANA_CLIENT_SECRET"
```

### API を読む

```bash
asana-cli me
asana-cli --output table me
asana-cli workspaces list
asana-cli --output table workspaces list
asana-cli workspaces ls
asana-cli projects list 123
asana-cli --output table projects list 123
asana-cli projects ls --workspace 123
asana-cli tasks list 456
asana-cli --output table tasks list 456
asana-cli tasks ls --project 456
asana-cli sections list 456
asana-cli --output table sections list 456
asana-cli tasks get 789
asana-cli --output compact tasks get 789
asana-cli tasks subtasks 789
asana-cli tasks stories 789
asana-cli --output table tasks comments 789
asana-cli tasks comments 789
asana-cli tasks attachments 789
```

補足:
- `tasks stories` は task の story 履歴全体を返しますが、Asana API の compact record が中心です。
- `tasks comments` は `comment_added` の story だけを抽出し、本文表示に必要な `text` / `html_text` / `created_at` / `created_by.name` を含めて返します。
- コメント本文を確認したい場合は `tasks comments` を使ってください。

### task と subtask を操作する

workspace、project、親 task、または project/section membership を指定して task を作成できます。

```bash
asana-cli tasks create --workspace 123 --name "リリース準備" --due-on 2026-08-31
asana-cli tasks create --project 456 --name "文面レビュー" --follower 111 --tag 222
asana-cli tasks create-subtask 789 --name "リンク確認"
asana-cli tasks update 789 --completed true
```

task field には `--notes` / `--html-notes`、`--assignee`、`--completed`、
`--approval-status`、`--resource-subtype`、`--start-on` / `--start-at`、
`--due-on` / `--due-at`、複数指定できる `--follower`、`--project`、`--tag`、
`--custom-field GID=VALUE`、`--membership PROJECT_GID=SECTION_GID` があります。
本文や日付の排他的な形式は API を呼ぶ前に検証します。update では明示した field だけを送信します。

```bash
asana-cli tasks set-parent 789 --parent 123 --insert-after 456
asana-cli tasks unset-parent 789
```

### task を一覧・検索する

list context は必ず1つだけ指定します。

```bash
asana-cli tasks list --project 123
asana-cli tasks list --section 234
asana-cli tasks list --tag 345
asana-cli tasks list --user-task-list 456
asana-cli tasks search --workspace 123 --assignee me --projects-any 456,789 --sort-by due_date --limit 50
asana-cli tasks get-custom-id OPS-42 --workspace 123
```

list、search、get-custom-id では `--opt-fields` で追加 field を指定できます。
workspace task 検索には Asana Premium が必要です。検索結果は eventual consistency であり、
通常の list command で使う offset pagination は利用できません。`--limit` は最大100件です。
手動で続き取得する場合は作成時刻で sort し、処理済み item を後続 query から除外します。

### section と project 配置を操作する

```bash
asana-cli sections get 123
asana-cli sections create --project 456 --name "進行中"
asana-cli sections update 123 --name "対応中"
asana-cli sections tasks 123
asana-cli sections add-task 123 --task 789
asana-cli sections move 123 --project 456 --after-section 111
asana-cli tasks add-project 789 --project 456 --section 123
asana-cli tasks remove-project 789 --project 456
```

`--insert-before` と `--insert-after`（section move の対応 flag）は同時指定できません。
Asana では空の section だけを削除でき、project の最後の section は削除できません。

### comment と task 関連付けを操作する

```bash
asana-cli tasks comment 789 --text "レビューできます"
asana-cli stories get 123
asana-cli stories update 123 --text "更新後のコメント"
asana-cli stories delete 123
asana-cli tasks dependencies 789
asana-cli tasks add-dependencies 789 --dependency 111 --dependency 222
asana-cli tasks remove-dependencies 789 --dependency 111
asana-cli tasks add-dependents 789 --dependent 333
asana-cli tasks add-tag 789 --tag 444
asana-cli tasks add-followers 789 --follower 555 --follower 666
```

編集できるのは comment story だけです。`--text` と `--html-text` は同時指定できません。
Asana が batch body を受け付ける関連付けでは GID flag を複数回指定できます。
dependency/dependent 合計上限を含む API 制限エラーは Asana のメッセージを表示します。

### project と membership を操作する

```bash
asana-cli projects get 123
asana-cli projects create --workspace 456 --name "リリース"
asana-cli projects update 123 --privacy-setting private
asana-cli projects tasks 123
asana-cli tasks projects 789
asana-cli workspaces projects 456
asana-cli workspaces create-project 456 --name "Workspace project"
asana-cli teams projects 789
asana-cli teams create-project 789 --name "Team project"
asana-cli memberships create --parent 123 --member 456 --access-level editor
asana-cli memberships list --parent 123
asana-cli memberships update 789 --access-level commenter
asana-cli memberships delete 789
asana-cli projects add-followers 123 --follower 456
asana-cli projects task-counts 123
```

新しい project 共有には deprecated な `team` field ではなく membership を使います。
member には user または team を指定できます。project task count には Asana の追加 rate/cost limit があるため、頻繁な polling は避けてください。

### attachment

既存の `tasks attachments TASK_GID` は維持しつつ、公式の
`GET /attachments?parent=...` route を使うようになりました。parent 指定の command は
task、project、project brief に対応します。

```bash
asana-cli attachments list 789
asana-cli attachments get 123
asana-cli attachments upload --parent 789 --file ./report.pdf
asana-cli attachments upload --parent 789 --url https://example.com/report --name "レポート"
asana-cli attachments delete 123
```

file upload の上限は100MBです。非ASCII filename は UTF-8 の multipart filename として送信します。
ローカル file の内容や access token を CLI 出力や API error text に含めません。

### 複製・template 化・削除・job

```bash
asana-cli tasks duplicate 789 --name "task のコピー"
asana-cli projects duplicate 123 --name "project のコピー"
asana-cli projects save-as-template 123 --name "リリース template" --public false --workspace 456
asana-cli jobs get JOB_GID
asana-cli tasks delete 789
asana-cli sections delete 123
asana-cli projects delete 456
```

duplicate と save-as-template は job GID を返します。成功・失敗は `jobs get` で確認します。
CLI は無期限 polling を行いません。削除した task は削除実行 user の Asana trash に入り、
30日以内なら復元できます。それ以降は Asana から完全に削除されます。

## 設定ファイル

既定パス:

```text
$XDG_CONFIG_HOME/asana-cli/credentials.json
~/.config/asana-cli/credentials.json
```

保存される内容:
- `clientId`
- `redirectUri`
- `token.access_token`
- `token.refresh_token`
- `token.token_type`
- `token.expires_in`
- `token.expires_at`

保存しない内容:
- `clientSecret`

必要なら `--config /path/to/credentials.json` で変更できます。

## 環境変数

- `ASANA_API_BASE`: Asana API base URL を上書き
- `ASANA_OAUTH_TOKEN_ENDPOINT`: OAuth token endpoint を上書き
- `BROWSER`: `auth login` で使うブラウザコマンド
- `ASANA_CLIENT_SECRET`: 保存済み access token が期限切れまたは期限間近の場合、API コール前に自動 refresh を有効化する。値は永続化されません。

### 自動 token refresh

`ASANA_CLIENT_SECRET` を設定すると、すべての API コマンド実行前に保存済み access token の有効期限を確認します。期限切れまたは残り5分以内の場合、自動的に refresh して新しい token を設定ファイルに保存します。refresh に失敗した場合は API コールを行わずエラーで終了します。

## Skills

AI Agent からこの CLI を扱うための Skill は `skills/` に置いています。

現在含まれるもの:
- `skills/asana-cli-operator/`
  - `asana-cli` の運用 Skill。認証状態確認、workspace / project / task / comment / attachment の取得、token refresh、出力形式の使い分けを定義しています。
  - 本体: `skills/asana-cli-operator/SKILL.md`

詳細は `skills/README.md` を参照してください。

## 開発

```bash
gofmt -w cmd/asana-cli/main.go internal/**/*.go
go test ./...
go vet ./...
go build -o /tmp/asana-cli ./cmd/asana-cli
```

安全な smoke check:

```bash
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```

### Git hooks (lefthook)

このプロジェクトは [lefthook](https://github.com/evilmartians/lefthook) を使って、コミット前にフォーマットとリントを実行しています。初回のみ以下を実行してください：

```bash
# lefthook のインストール（Go toolchain が必要）
go install github.com/evilmartians/lefthook@latest

# このリポジトリで hooks を有効化
lefthook install
```

有効化後、コミット時に以下のチェックが自動実行されます：

- `gofmt` — Go ファイルのフォーマット確認
- `go vet` — 静的解析の実行
- `go test` — テストスイートの実行

## GitHub Actions

- `ci.yml`: gofmt check / vet / test
- `release.yml`: タグ push で macOS / Linux バイナリをビルドして release asset を作成

## 開発フロー

- `main` は protected branch として扱い、直接 push しない
- 変更は feature branch で行い、Pull Request 経由で `main` に取り込む
- 可能なら squash merge を使い、不要になった branch は削除する
