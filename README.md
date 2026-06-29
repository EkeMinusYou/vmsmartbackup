# vmsmartbackup

`vmsmartbackup` は、VictoriaMetrics の `vmbackup` コマンドを [スマートバックアップ](https://docs.victoriametrics.com/vmbackup/#smart-backups) の方針に則って定期実行するサービスです。

Kubernetes 上で `vmstorage` のサイドカーとして動かすことを想定していますが、必要な環境変数を渡せば単体でも動作します。

## 仕組み

[スマートバックアップ](https://docs.victoriametrics.com/vmbackup/#smart-backups) に従い、2 種類のジョブを [gocron](https://github.com/go-co-op/gocron) で定期実行します。

| ジョブ | スケジュール (cron) | 内容 |
| --- | --- | --- |
| hourly | `0 * * * *`（毎時 0 分） | ストレージのスナップショットを作成し `latest/` へバックアップ |
| daily | `30 20 * * *`（UTC、= JST 05:30） | `latest/` から `daily/<YYYYMMDD>/` へコピー |

バックアップ先（Google Cloud Storage）のディレクトリ構成は以下の通りです。

```
<bucket>/
├── latest/
│   └── <vmstorage-name>
└── daily/
    └── <YYYYMMDD>/
        └── <vmstorage-name>
```

`<vmstorage-name>` は環境変数 `POD_NAME` の末尾（`-` 区切りの最後の要素）を使い、`vmstorage-<index>` という形式で決定されます。
例: `POD_NAME=my-cluster-vmstorage-0` → `vmstorage-0`

## 環境変数

| 変数 | 必須 | 説明 |
| --- | --- | --- |
| `VMSTORAGE_URL` | ✓ | vmstorage のベース URL。起動時の `/health` 待機と、スナップショット作成（`/snapshot/create`）に使用します。例: `http://localhost:8482` |
| `STORAGE_DATA_PATH` | ✓ | vmstorage のデータディレクトリ。例: `/storage` |
| `BACKUP_BUCKET_NAME` | ✓ | バックアップ先の GCS バケット名 |
| `POD_NAME` | ✓ | 実行 Pod 名。末尾の要素から `vmstorage-<index>` を導出します |

GCS への認証は `vmbackup` が [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials) を利用します。必要に応じて `GOOGLE_APPLICATION_CREDENTIALS` を設定してください。

## ヘルスチェック

ポート `8000` で `/proxy-healthz` エンドポイントを公開します。

## Kubernetes デプロイ例

`vmstorage` のサイドカーとして動かす例です。VictoriaMetrics を Helm チャートで管理している場合は、values の `vmstorage.extraContainers` に以下を追記します（`<your-registry>` や `<your-backup-bucket>` は環境に合わせて置き換えてください）。

```yaml
vmstorage:
  extraContainers:
    - name: vmsmartbackup
      image: <your-registry>/vmsmartbackup:latest
      imagePullPolicy: Always
      readinessProbe:
        httpGet:
          path: /proxy-healthz
          port: 8000
        initialDelaySeconds: 3
        timeoutSeconds: 3
      livenessProbe:
        httpGet:
          path: /proxy-healthz
          port: 8000
        initialDelaySeconds: 3
        periodSeconds: 3
      env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: STORAGE_DATA_PATH
          value: /storage
        - name: VMSTORAGE_URL
          value: http://localhost:8482
        - name: BACKUP_BUCKET_NAME
          value: <your-backup-bucket>
      volumeMounts:
        - name: vmstorage-volume
          mountPath: /storage
```

ポイント:

- `POD_NAME` は Downward API（`metadata.name`）で渡し、ここから `vmstorage-<index>` を導出します。
- `vmstorage` 本体と同じ data volume（例: `vmstorage-volume`）を `/storage` にマウントし、`STORAGE_DATA_PATH` と一致させます。
- GCS への認証は別途 Application Default Credentials（Workload Identity やサービスアカウントキー等）を設定してください。

## ビルド・実行

```shell
# ローカルビルド
make build        # -> build/app

# ローカル実行
make run

# テスト
make test

# Docker イメージのビルド
make image

# レジストリへ push
make push REGISTRY=<your-registry>
```

`vmbackup` バイナリは Docker イメージのビルド時に [VictoriaMetrics のリリース](https://github.com/VictoriaMetrics/VictoriaMetrics/releases) から取得します。バージョンは `Dockerfile` の `VMUTILS_VERSION` で指定します。
