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
)

func main() {
	vmstorageURL := os.Getenv("VMSTORAGE_URL") + "/health"
	if err := util.WaitUntilReady(context.Background(), vmstorageURL); err != nil {
		log.Fatalf("vmstorage is not ready: %s", err)
	}

	log.Printf("Starting vmsmartbackup service")

	scheduler, err := gocron.NewScheduler()
	if err != nil {
		log.Fatalf("Failed to create scheduler: %v", err)
	}

	// 毎時のバックアップジョブを追加（1時間ごと）
	_, err = scheduler.NewJob(
		gocron.CronJob("0 * * * *", false),
		gocron.NewTask(service.RunHourlyBackup),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		log.Fatalf("Failed to create hourly job: %v", err)
	}
	log.Print("Hourly backup job scheduled")

	// 毎日のバックアップジョブを追加（毎日JST 05:30 = UTC 20:00）
	_, err = scheduler.NewJob(
		gocron.CronJob("30 20 * * *", false),
		gocron.NewTask(service.RunDailyBackup),
	)
	if err != nil {
		log.Fatalf("Failed to create daily job: %v", err)
	}
	log.Print("Daily backup job scheduled")

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
