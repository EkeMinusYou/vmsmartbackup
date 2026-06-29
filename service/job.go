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

// RunLatestBackup はストレージのスナップショットを作成し、常に最新化される latest/ へバックアップする。
func RunLatestBackup() {
	log.Printf("Running latest backup from storage to latest/")

	cmd := exec.Command(
		"/vmbackup",
		"-storageDataPath="+storageDataPath,
		"-snapshot.createURL="+snapshotURL,
		"-dst="+fmt.Sprintf("gs://%s/latest/%s", backupBucketName, vmstorageName),
		"-loggerFormat=json",
		"-loggerJSONFields=ts:timestamp,level:severity,msg:message", // Cloud Loggingのjson形式でログを出力
	)

	log.Printf("Latest backup command: %s", cmd.String())
	err := startCommand(cmd)

	if err != nil {
		log.Printf("Latest backup failed: %v", err)
		return
	}
	log.Printf("Latest backup completed successfully")
}

// RunSnapshotBackup は latest/ を日付付きディレクトリ（snapshot/<YYYYMMDD>/）へコピーし、静的な世代として保管する。
func RunSnapshotBackup() {
	log.Printf("Running snapshot backup from latest/ to snapshot/")

	currentDate := time.Now().Format("20060102")

	cmd := exec.Command(
		"/vmbackup",
		"-origin="+fmt.Sprintf("gs://%s/latest/%s", backupBucketName, vmstorageName),
		"-dst="+fmt.Sprintf("gs://%s/snapshot/%s/%s", backupBucketName, currentDate, vmstorageName),
		"-loggerFormat=json",
		"-loggerJSONFields=ts:timestamp,level:severity,msg:message", // Cloud Loggingのjson形式でログを出力
	)

	log.Printf("Snapshot backup command: %s", cmd.String())
	err := startCommand(cmd)

	if err != nil {
		log.Printf("Snapshot backup failed: %v", err)
		return
	}
	log.Printf("Snapshot backup completed successfully")
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
