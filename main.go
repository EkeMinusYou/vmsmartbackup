package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-co-op/gocron/v2"

	"github.com/EkeMinusYou/vmsmartbackup/service"
	"github.com/EkeMinusYou/vmsmartbackup/service/util"
)

const (
	healthCheckPort = "8000"

	// バックアップ頻度のデフォルト（cron 形式、UTC）。環境変数で上書き可能。
	defaultLatestBackupCron   = "0 * * * *"   // 毎時 0 分
	defaultSnapshotBackupCron = "30 20 * * *" // 毎日 UTC 20:30（= JST 05:30）
)

func main() {
	vmstorageURL := os.Getenv("VMSTORAGE_URL") + "/health"
	if err := util.WaitUntilReady(context.Background(), vmstorageURL); err != nil {
		log.Fatalf("vmstorage is not ready: %s", err)
	}

	log.Printf("Starting vmsmartbackup service")

	latestBackupCron := getenvDefault("LATEST_BACKUP_CRON", defaultLatestBackupCron)
	snapshotBackupCron := getenvDefault("SNAPSHOT_BACKUP_CRON", defaultSnapshotBackupCron)

	scheduler, err := gocron.NewScheduler()
	if err != nil {
		log.Fatalf("Failed to create scheduler: %v", err)
	}

	// ストレージから latest/ へのバックアップジョブを追加
	_, err = scheduler.NewJob(
		gocron.CronJob(latestBackupCron, false),
		gocron.NewTask(service.RunLatestBackup),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		log.Fatalf("Failed to create latest backup job (cron %q): %v", latestBackupCron, err)
	}
	log.Printf("Latest backup job scheduled (cron %q)", latestBackupCron)

	// latest/ から snapshot/ へのバックアップジョブを追加
	_, err = scheduler.NewJob(
		gocron.CronJob(snapshotBackupCron, false),
		gocron.NewTask(service.RunSnapshotBackup),
	)
	if err != nil {
		log.Fatalf("Failed to create snapshot backup job (cron %q): %v", snapshotBackupCron, err)
	}
	log.Printf("Snapshot backup job scheduled (cron %q)", snapshotBackupCron)

	scheduler.Start()
	log.Print("Scheduler started")

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy-healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{}"))
	})
	log.Printf("Starting health check server on :%s", healthCheckPort)
	if err := http.ListenAndServe(":"+healthCheckPort, mux); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

// getenvDefault は環境変数 key の値を返す。未設定または空の場合は fallback を返す。
func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
