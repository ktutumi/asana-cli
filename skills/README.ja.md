# skills

言語: [English](README.md) | 日本語

このディレクトリには、このリポジトリの CLI を AI Agent が安全かつ一貫して扱うための Skill を置きます。

目的:
- 実コマンド実行時の手順を標準化する
- 認証や出力形式の扱いを統一する
- 実装知識と運用知識を分離する

含まれる Skill:
- `asana-cli-operator/`
  - `asana-cli` を使って認証状態の確認、workspace / project / task / comment / attachment 取得、token refresh などを行うための運用 Skill

使い分け:
- CLI 自体を実装・修正する場合は、コードとテストを読む
- CLI を実際に使って確認・取得する場合は、このディレクトリの Skill を使う

現在の構成:
```text
skills/
  README.md
  asana-cli-operator/
    SKILL.md
```

補足:
- `asana-cli-operator` はまず `auth status` を確認してから API read を行う前提です
- comment 本文が必要な場合は `tasks stories` ではなく `tasks comments` を優先します
- localhost OAuth login と OOB/manual flow は別物として扱います
