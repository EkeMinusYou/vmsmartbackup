# vmsmartbackup

`vmsmartbackup` は、VictoriaMetrics の `vmbackup` コマンドを [スマートバックアップ](https://docs.victoriametrics.com/vmbackup/#smart-backups) の方針に則って定期実行するサービスです。

Kubernetes 上で `vmstorage` のサイドカーとして動かすことを想定していますが、必要な環境変数を渡せば単体でも動作します。

## 仕組み

[スマートバックアップ](https://docs.victoriametrics.com/vmbackup/#smart-backups) に従い、2 種類のジョブを [gocron](https://github.com/go-co-op/gocron) で定期実行します。

| ジョブ | 役割 | デフォルトのスケジュール (cron) | 内容 |
| --- | --- | --- | --- |
| latest backup | 最新状態の保持 | `0 * * * *`（毎時 0 分） | ストレージのスナップショットを作成し、常に最新化される `latest/` へバックアップ |
| snapshot backup | 世代の保管 | `30 20 * * *`（UTC、= JST 05:30） | `latest/` から日付付きの `snapshot/<YYYYMMDD>/` へコピーし、静的な世代として残す |

- **latest backup**: `latest/` を常に上書きして「最新の1世代」を維持します。
- **snapshot backup**: 実行時点の `latest/` を日付ディレクトリに固定保存し、複数世代を残します。

> [!NOTE]
> snapshot backup は `latest/` を入力にコピーを作るため、先に latest backup が実行されて `latest/` が作成されている必要があります。初回起動直後に snapshot backup が走るとコピー元が無く失敗するので、最初の latest backup 実行以降が有効なバックアップになります。

バックアップ先（Google Cloud Storage）のディレクトリ構成は以下の通りです。

```
<bucket>/
├── latest/
│   └── <vmstorage-name>
└── snapshot/
    └── <YYYYMMDD>/
        └── <vmstorage-name>
```

`<vmstorage-name>` は環境変数 `POD_NAME` の末尾（`-` 区切りの最後の要素）を使い、`vmstorage-<index>` という形式で決定されます。
例: `POD_NAME=my-cluster-vmstorage-0` → `vmstorage-0`

## 前提条件

- バックアップ先の **GCS バケットを事前に作成**しておくこと（このサービスはバケット自体は作成しません）。
- バックアップを実行する主体（GKE なら Workload Identity に紐づくサービスアカウント、単体実行なら ADC のアカウント）に、対象バケットへの読み書き権限（例: `roles/storage.objectAdmin`）を付与しておくこと。
- イメージをビルドする場合は Docker、ローカルでビルド・実行する場合は Go 1.26 以上。

## 環境変数

| 変数 | 必須 | 説明 |
| --- | --- | --- |
| `VMSTORAGE_URL` | ✓ | vmstorage のベース URL。起動時の `/health` 待機と、スナップショット作成（`/snapshot/create`）に使用します。例: `http://localhost:8482` |
| `STORAGE_DATA_PATH` | ✓ | vmstorage のデータディレクトリ。例: `/storage` |
| `BACKUP_BUCKET_NAME` | ✓ | バックアップ先の GCS バケット名 |
| `POD_NAME` | ✓ | 実行 Pod 名。末尾の要素から `vmstorage-<index>` を導出します。単体実行時も `xxx-vmstorage-0` のように末尾を合わせて指定します |
| `LATEST_BACKUP_CRON` | | latest backup（ストレージ→`latest/`）の実行頻度（cron 形式）。デフォルト `0 * * * *`（毎時 0 分） |
| `SNAPSHOT_BACKUP_CRON` | | snapshot backup（`latest/`→`snapshot/`）の実行頻度（cron 形式）。デフォルト `30 20 * * *`（UTC、= JST 05:30） |

> [!NOTE]
> cron は UTC で評価されます。`LATEST_BACKUP_CRON` / `SNAPSHOT_BACKUP_CRON` で各ジョブの頻度を任意に変更できます（例: `latest/` の更新を 30 分ごとにするなら `LATEST_BACKUP_CRON="*/30 * * * *"`）。

GCS への認証は `vmbackup` が [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials) を利用します。単体実行で明示的に鍵を使う場合は `GOOGLE_APPLICATION_CREDENTIALS` を設定してください。

## 導入手順（Kubernetes サイドカー）

`vmstorage` のサイドカーとして動かす、想定している主な使い方です。

### 1. イメージをビルドして push する

```shell
make push REGISTRY=<your-registry>
# 例: make push REGISTRY=asia-northeast1-docker.pkg.dev/<project>/<repo>
```

### 2. バケットと権限を用意する

[前提条件](#前提条件) の通り、バックアップ先バケットを作成し、実行サービスアカウントへ書き込み権限を付与します。

### 3. vmstorage に追記する

VictoriaMetrics を Helm チャートで管理している場合は、values の `vmstorage.extraContainers` に以下を追記します（`<your-registry>` / `<your-backup-bucket>` は環境に合わせて置き換え）。

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
- GCS への認証は Workload Identity 等で対象バケットへの書き込み権限を持たせてください。

### 4. 動作を確認する

デプロイ後、サイドカーのログにジョブのスケジュール登録が出ていること、毎時 0 分以降に `gs://<bucket>/latest/<vmstorage-name>/` が作成されることを確認します。

```shell
kubectl logs <vmstorage-pod> -c vmsmartbackup
gcloud storage ls gs://<your-backup-bucket>/latest/
```

## 単体で実行する

Kubernetes を使わず、ローカルや任意のホストで動かすこともできます。`vmstorage` に到達でき、GCS への認証が通る環境であれば動作します。

Docker の場合:

```shell
docker run --rm \
  -e VMSTORAGE_URL=http://vmstorage:8482 \
  -e STORAGE_DATA_PATH=/storage \
  -e BACKUP_BUCKET_NAME=<your-backup-bucket> \
  -e POD_NAME=local-vmstorage-0 \
  -e GOOGLE_APPLICATION_CREDENTIALS=/secrets/key.json \
  -v /path/to/vmstorage-data:/storage \
  -v /path/to/key.json:/secrets/key.json:ro \
  -p 8000:8000 \
  <your-registry>/vmsmartbackup:latest
```

ローカルの Go から動かす場合（`vmbackup` バイナリが PATH 上に必要）:

```shell
export VMSTORAGE_URL=http://localhost:8482
export STORAGE_DATA_PATH=/storage
export BACKUP_BUCKET_NAME=<your-backup-bucket>
export POD_NAME=local-vmstorage-0
make run
```

## リストア

バックアップからの復元は VictoriaMetrics の [`vmrestore`](https://docs.victoriametrics.com/vmrestore/) を使います（`vmbackup` と同じ `vmutils` に同梱）。**リストア対象の `vmstorage` を停止した状態**で実行してください。

```shell
# snapshot バックアップ（日付付き世代）から復元する例
vmrestore \
  -src=gs://<your-backup-bucket>/snapshot/<YYYYMMDD>/vmstorage-0 \
  -storageDataPath=/storage

# latest バックアップ（最新状態）から復元する場合
vmrestore \
  -src=gs://<your-backup-bucket>/latest/vmstorage-0 \
  -storageDataPath=/storage
```

復元後に `vmstorage` を起動し直します。

## ヘルスチェック

ポート `8000` で `/proxy-healthz` エンドポイントを公開します（上記マニフェストの readiness/liveness probe で使用）。

## ビルド・実行（Make ターゲット）

```shell
make build                  # ローカルビルド -> build/app
make run                    # ローカル実行（要・環境変数）
make test                   # テスト
make image                  # Docker イメージのビルド
make push REGISTRY=<reg>    # イメージのビルドとレジストリへの push
```

`vmbackup` バイナリは Docker イメージのビルド時に [VictoriaMetrics のリリース](https://github.com/VictoriaMetrics/VictoriaMetrics/releases) から取得します。バージョンは `Dockerfile` の `VMUTILS_VERSION` で指定します。
