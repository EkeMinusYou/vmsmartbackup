package service

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	vmstorageName    string
	storageDataPath  string
	snapshotURL      string
	backupBucketName string
)

func init() {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		log.Fatalf("POD_NAME is not set")
	}
	parts := strings.Split(podName, "-")
	vmstorageName = "vmstorage-" + parts[len(parts)-1]

	storageDataPath = os.Getenv("STORAGE_DATA_PATH")
	if storageDataPath == "" {
		log.Fatalf("STORAGE_DATA_PATH is not set")
	}

	vmstorageURL := os.Getenv("VMSTORAGE_URL")
	if vmstorageURL == "" {
		log.Fatalf("VMSTORAGE_URL is not set")
	}
	snapshotURL = vmstorageURL + "/snapshot/create"
	backupBucketName = os.Getenv("BACKUP_BUCKET_NAME")
	if backupBucketName == "" {
		log.Fatalf("BACKUP_BUCKET_NAME is not set")
	}
}

// ストレージから latest へのバックアップ（毎時実行）
func RunHourlyBackup() {
	log.Printf("Running hourly backup from storage to latest")

	cmd := exec.Command(
		"/vmbackup",
		"-storageDataPath="+storageDataPath,
		"-snapshot.createURL="+snapshotURL,
		"-dst="+fmt.Sprintf("gs://%s/latest/%s", backupBucketName, vmstorageName),
		"-loggerFormat=json",
		"-loggerJSONFields=ts:timestamp,level:severity,msg:message", // Cloud Loggingのjson形式でログを出力
	)

	log.Printf("Hourly backup command: %s", cmd.String())
	err := startCommand(cmd)

	if err != nil {
		log.Printf("Hourly backup failed: %v", err)
		return
	}
	log.Printf("Hourly backup completed successfully")
}

// latest からデイリーディレクトリへのバックアップ（毎日実行）
func RunDailyBackup() {
	log.Printf("Running daily backup from latest to daily")

	currentDate := time.Now().Format("20060102")

	cmd := exec.Command(
		"/vmbackup",
		"-origin="+fmt.Sprintf("gs://%s/latest/%s", backupBucketName, vmstorageName),
		"-dst="+fmt.Sprintf("gs://%s/daily/%s/%s", backupBucketName, currentDate, vmstorageName),
		"-loggerFormat=json",
		"-loggerJSONFields=ts:timestamp,level:severity,msg:message", // Cloud Loggingのjson形式でログを出力
	)

	log.Printf("Daily backup command: %s", cmd.String())
	err := startCommand(cmd)

	if err != nil {
		log.Printf("Daily backup failed: %v", err)
		return
	}
	log.Printf("Daily backup completed successfully")
}

// コマンドを実行してstdoutを一行ずつログに出力
func startCommand(cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 2)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
		}
		if err := scanner.Err(); err != nil {
			done <- err
			return
		}
		done <- nil
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
		}
		if err := scanner.Err(); err != nil {
			done <- err
			return
		}
		done <- nil
	}()

	for range 2 {
		if err := <-done; err != nil {
			return err
		}
	}

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
