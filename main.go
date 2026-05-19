package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)


// env is a temporary development environment
type Env struct {
	ID string `json:"id"` // name of docker container
	Container string `json:"-`
	CreatedAt time.Time `json:"createdAt"` // start time to add constraints
	LastPing time.Time `json:"last_ping"` // to check for disconnection
	TunnelURL string `json:"tunnel_url,omitempty"`// to store either cloudflare tunnel url or empty if cloudflared is not init yet
	TunnelPID int `json:"-"` // obvious
}

// var makes global variable

var (
	envs = sync.Map{} // id -> *Env , basically map of envs
	upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request)bool {return true}} // returns true if the origin of the request is allowed, which is always true in this case
	rateLimits = sync.Map{} // ip -> count , storing ip and count of requests from that ip
	logMutex = sync.Mutex{} // mutex means mutual exclusion , which is used to prevent race conditions , in this case we need it to make sure only one request is accessing the log file at a time
)

// http routes
func main(){
	http.HandleFunc("/env",handleCreateEnv)
	http.HandleFunc("/env/",handleEnvAction)
	http.HandleFunc("/ws/env/", handleShell)
	http.HandleFunc("/expose",handleExpose)
	http.HandleFunc("/envs",handleListEnvs)
	http.Handle("/",http.FileServer(http.Dir("./public")))

	// background goroutines
	go cleanupLoop()
	go abuseDetectLoop()
	go rateLimitResetLoop()

	log.Println(" starting on :8080")
	log.Fatal(http.ListenAndServe(":8080",nil))
}

// helpers
func generateID() string{
	b:= make([]byte,4) // makes a byte slice of 4 bytes
	rand.Read(b) // reads random bytes into the slice
	return hex.EncodeToString(b) // returns the hexadecimal representation of the bytes
}

func sanitizeEnvID(id string) bool{
	if len(id)==0 || len(id) > 16{
		return false
	}
	for _,c := range id {
		if !((c>='a' && c<='z') || (c>='0' && c<='9')){
			return false // what this does is it checks if the character is a lowercase letter or a number and if yes then return true and if not then return false and loops for all the characters
		}
	}
	return true
}

func extractIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = strings.Split(r.RemoteAddr,":")[0]
	}
	return ip  // ths whole fucntion returns ip by checking the forwarded for header first and if not found then the real ip header and if not found then the remote address and if not found then the remote address
}

func logEvent(eventType,envID,detail string){
	logMutex.Lock()// locked the mutex so only one request is accessing the log file at a time
	defer logMutex.Unlock()// unlocked the mutex so only one request is accessing the log file at a time
	
	logEntry:= map[string]interface{}{
		"timestamp":time.Now().Format(time.RFC3339),
		"type": eventType,
		"envID": envID,
		"detail": detail,
	}

	data,_ := json.Marshal(logEntry) // data variable will store the JSON representation of the logEntry map , and the underscore means we are ignoring the error that is returned by the json.Marshal function

	file,err:= os.OpenFile("events.log", os.O_APPEND| os.O_CREATE|os.O_WRONLY,0644) // in order what all these do is : O_APPEND appends new data , O_CREATE creats file if doesnt exist , O_WRONLY opens file in read only , 0644 sets file permission owner can read write ; group and other can only read.

	if err!=nil {
		log.Printf("Failed to log event %s: %v\n",eventType,err)
		return
	}
	defer file.Close()

	file.Write(append(data,'\n'))
	// fmt.Fprintln(file,string(data))
}

func formatDuration(d time.Duration) string {
	total := int(d.Seconds())
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
 
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
} // this function formats the duration in the format of "HHhMMm" and returns it

// =====================================
//	ENV CREATION AND MANAGEMENT
// =====================================

func handleCreateEnv(w http.ResponseWriter,r *http.Request){
	if r.Method != "POST"{
		http.Error(w,"Method not allowed",http.StatusMethodNotAllowed)
		return
	}

	ip:= extractIP(r)

	// rate limits 5 per hour per ip
	val,_ := rateLimits.LoadOrStore(ip,0)
	count := val.(int)

	// TODO : TODO
	
}