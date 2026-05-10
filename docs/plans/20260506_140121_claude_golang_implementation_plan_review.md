# Go 実装計画レビュー

## 総評

計画の大枠は、プロジェクト指示とよく整合しています。特に、Go CLI の package 境界、標準ライブラリ中心の方針、hermetic test、secret handling、OAuth callback security は妥当です。

一方で、実装前に必ず解消すべき点があります。特に **`compact` 出力仕様の不一致** は、テスト・README・互換性レビューで期待値が割れる可能性があるため、最優先で確定してください。

## 良い点

### 1. package 構成がプロジェクト指示と整合している

計画の大枠は、AGENTS の Go CLI 構成と整合しています。

- `cmd/asana-cli/main.go`
- `internal/cli`
- `internal/asana`
- `internal/oauth`
- `internal/config`

これらへ責務を分ける方針は、プロジェクト概要の package 境界（`AGENTS.md:7-15`）および CLI flow（`AGENTS.md:55-65`）と一致します。

Plan でも同じファイル群を Task 1 / 4 / 5 / 6 / 7 / 8 / 10 / 11 に分解しています（`plan.md:9-74`）。

### 2. 標準ライブラリ中心の方針が妥当

標準ライブラリ中心で実装し、依存追加を避ける方針は妥当です。

AGENTS は CLI framework の追加を避けるよう求めています（`AGENTS.md:21-23`）。Plan も軽量フラグパーサを自前実装する前提です（`plan.md:22-26`）。

### 3. テスト戦略が概ね十分

テスト戦略は概ね十分です。

- `httptest.Server`
- `t.TempDir()`
- live API を使わない smoke checks

これらを使う方針は、AGENTS の hard rules / testing guidance（`AGENTS.md:19-25`, `AGENTS.md:99-105`）と一致しています。

Plan でも token / API / config / callback / 最終検証で hermetic な受け入れ条件を置いています（`plan.md:31-32`, `plan.md:43-50`, `plan.md:55-56`, `plan.md:97-98`）。

### 4. secret handling と OAuth callback security が明示されている

実 secret / token / code を出力しないこと、token stdout を redaction することが計画に明示されています（`plan.md:67-68`, `plan.md:136`）。

これは AGENTS の security checklist（`AGENTS.md:114-123`）と整合します。

## 修正済みの点

なし。

今回は計画レビューのみです。ファイル修正は、レビュー結果の出力ファイル作成だけです。

## Blocker

### `compact` 出力仕様がプロジェクト指示と衝突している

Plan では、collection の `compact` 出力を「header なし TSV 行」としています（`plan.md:61`、risk でも同趣旨 `plan.md:137`）。

一方で、AGENTS と skill は `compact` を `field=value` lines と規定しています（`AGENTS.md:107-112`, `.claude/skills/asana-cli-go-development/SKILL.md:124-130`）。

このまま実装すると、次の期待値が割れる可能性があります。

- CLI テスト
- README の説明
- 互換性レビュー
- 実装者間の認識

実装前に、次のどちらを採用するか 1 つに確定してください。

1. Rust 互換を優先し、collection compact は TSV にする
2. 現行プロジェクト指示どおり、compact は常に `field=value` にする

確定後、Plan / AGENTS / テスト期待値を同じ仕様に揃える必要があります。

## Notes

### 1. module path は未決事項にしない方がよい

Plan では module path が「第一候補」かつ未決リスク扱いです（`plan.md:13`, `plan.md:133`）。

しかし AGENTS は Project を `github.com/ktutumi/asana-cli-go` と明記しています（`AGENTS.md:7`）。

実装計画では未決事項にせず、次の内容をこの module path に固定する方が安全です。

- `go.mod`
- README の `go install` 例

### 2. README / CI / release は新規作成の可能性を明記する

repo 現状を `git ls-files` で確認したところ、root `README.md` と `.github/workflows/*.yml` は tracked file として存在しませんでした。

一方で、Plan は README / CI / release を「Modify」としています（`plan.md:76-91`, `plan.md:100-104`）。

実装時に迷わないよう、これらは次のように明記すると安全です。

> 存在しなければ新規作成する。

### 3. redirect URI の query / fragment 拒否テストを acceptance に追加する

`auth login` の redirect URI 制限について、Plan の risk には query 付き拒否が書かれています（`plan.md:135`）。

しかし Task 10 acceptance には、query / fragment 拒否テストが明示されていません（`plan.md:67-68`）。

skill は query / fragment 禁止を明確に要求しています（`.claude/skills/asana-cli-go-development/SKILL.md:117-122`）。

acceptance に次の拒否テストを追加すると、security gate が堅くなります。

- `http://127.0.0.1/callback?x=1`
- fragment 付き URI

### 4. `auth exchange` / `auth refresh` の flag contract を明文化する

Plan では、`auth exchange` / `auth refresh` の flag contract が高レベルに留まっています（`plan.md:64-68`）。

実装前に、次の required / optional matrix を acceptance に明文化すると安全です。

- `--client-id`
- `--client-secret`
- `--redirect-uri`
- `--code`
- `--refresh-token`

また、次の仕様もテストとして明示すると、設定互換・secret 非保存要件（`plan.md:31-32`, `plan.md:136`）をより確実に満たせます。

> `client_secret` は form body には使うが、設定ファイルには保存しない。
