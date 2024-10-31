package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	logFileName     = "website_monitor.log" // 日誌檔案名稱
	historyFileName = "status_history.json" // 歷史狀態檔案名稱
	interval        = 10 * time.Second      // 請求間隔時間
)

var urls = []string{
	"https://zerojudge.tw/",
	"http://srlb.somee.com/",
	"http://example.com/404",
	"http://10.255.255.1",
	"http://httpstat.us/403",
	"http://httpstat.us/502",
}

// statusText 根據狀態碼返回狀態碼的解釋
func statusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Found"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	default:
		return "Unknown Status"
	}
}

// WebsiteStatus 網站狀態結構
type WebsiteStatus struct {
	URL             string
	Status          int
	StatusMessage   string
	LastChecked     time.Time
	ResponseTime    time.Duration
	HistoryStatuses []HistoryStatus // 歷史狀態紀錄
}

// HistoryStatus 用於記錄歷史狀態的結構
type HistoryStatus struct {
	Status        int
	StatusMessage string
	CheckedTime   time.Time
	ResponseTime  time.Duration
}

// 變數，以存放目前網站狀態
var currentStatus = make(map[string]WebsiteStatus)

// 監聽網站狀態
func listenWebsiteStatus() {
	for {
		for _, url := range urls {
			start := time.Now()

			resp, err := http.Get(url)
			if err != nil {
				updateStatus(url, 0, "Connection Error", start, 0)
				log.Printf("Error checking %s: %v", url, err)
			} else {
				duration := time.Since(start)
				status := resp.StatusCode
				statusDescription := statusText(status)

				updateStatus(url, status, statusDescription, start, duration)
				resp.Body.Close()

				log.Printf("Checked %s - Status: %s, Response time: %v", url, statusDescription, duration)
			}

			time.Sleep(interval)
		}
	}
}

// 更新網站狀態
func updateStatus(url string, status int, statusMessage string, checkedTime time.Time, responseTime time.Duration) {
	// 檢查是否已經存在於狀態記錄中，如果不存在，則初始化
	if _, ok := currentStatus[url]; !ok {
		currentStatus[url] = WebsiteStatus{
			URL:           url,
			Status:        status,
			StatusMessage: statusMessage,
			LastChecked:   checkedTime,
			ResponseTime:  responseTime,
			HistoryStatuses: []HistoryStatus{
				{Status: status, StatusMessage: statusMessage, CheckedTime: checkedTime, ResponseTime: responseTime},
			},
		}
	} else {
		// 更新目前狀態，並將新狀態添加到歷史記錄中
		current := currentStatus[url]
		current.Status = status
		current.StatusMessage = statusMessage
		current.LastChecked = checkedTime
		current.ResponseTime = responseTime
		current.HistoryStatuses = append(current.HistoryStatuses, HistoryStatus{
			Status:        status,
			StatusMessage: statusMessage,
			CheckedTime:   checkedTime,
			ResponseTime:  responseTime,
		})
		currentStatus[url] = current
		saveHistoryToFile() // 保存歷史資料到檔案
	}
}

// 保存歷史資料到檔案
func saveHistoryToFile() {
	file, err := os.Create(historyFileName)
	if err != nil {
		log.Printf("Error creating history file: %v", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(currentStatus)
	if err != nil {
		log.Printf("Error encoding history to file: %v", err)
	}
}

// 從檔案讀取歷史資料
func loadHistoryFromFile() {
	file, err := os.Open(historyFileName)
	if err != nil {
		log.Printf("Error opening history file: %v", err)
		return
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&currentStatus)
	if err != nil {
		log.Printf("Error decoding history from file: %v", err)
	}
}

// 處理主頁請求
func indexHandler(w http.ResponseWriter, r *http.Request) {
	funcMap := template.FuncMap{
		"statusClass": func(status int) string {
			switch {
			case status == 200:
				return "status-ok"
			case status >= 400 && status < 500:
				return "status-warning"
			case status >= 500:
				return "status-error"
			default:
				return ""
			}
		},
		"toJson": toJson, // 註冊自定義 JSON 序列化函數
	}

	tmpl := template.Must(template.New("index.html").Funcs(funcMap).ParseFiles("index.html"))

	// 讀取當前網站狀態
	var websiteStatuses []WebsiteStatus
	for _, status := range currentStatus {
		websiteStatuses = append(websiteStatuses, status)
	}

	data := struct {
		WebsiteStatuses []WebsiteStatus
	}{
		WebsiteStatuses: websiteStatuses,
	}

	err := tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// toJson 是自定義的 JSON 序列化函數
func toJson(v interface{}) template.JS {
	js, err := json.Marshal(v)
	if err != nil {
		log.Printf("toJson error: %v", err)
		return template.JS("{}") // 返回一個空對象
	}
	return template.JS(js)
}

func main() {
	// 開啟或創建日誌檔案
	file, err := os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("無法開啟日誌檔案: %v", err)
	}
	defer file.Close()

	// 設置日誌輸出
	log.SetOutput(file)

	// 從檔案讀取歷史資料
	loadHistoryFromFile()

	// 啟動監聽網站狀態的協程
	go listenWebsiteStatus()

	// 設置靜態資源目錄，這裡假設有一個 index.html 作為模板
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/", indexHandler)

	// 監聽端口
	port := "8080"
	fmt.Printf("Starting server on port %s...\n", port)
	err = http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("無法啟動伺服器: %v", err)
	}
}
