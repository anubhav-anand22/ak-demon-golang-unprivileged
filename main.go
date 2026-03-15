// package unprivilegedgoakdemon

// import (
// 	"bufio"
// 	"fmt"
// 	"net"
// 	"os"
// )

// func main() {
// 	// Attempt to connect to the secure socket
// 	conn, err := net.Dial("unix", "/var/run/ble_bridge.sock")
// 	if err != nil {
// 		log.Printf("Error: %v\n", err)
// 		os.Exit(1)
// 	}
// 	defer conn.Close()

// 	// Invoke a test command
// 	fmt.Println("Sending PING to privileged process...")
// 	fmt.Fprintf(conn, `{"command": "PING"}`+"\n")

// 	// Read response
// 	resp, _ := bufio.NewReader(conn).ReadString('\n')
// 	log.Printf("Received: %s", resp)

// 	// Keep running so the parent doesn't restart us immediately
// 	select {}
// }

package main

import (
	"bufio"
	"path/filepath"
	"time"

	// "encoding/json"
	// "fmt"
	// "fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"os"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/anubhav-anand22/ak-demon-golang-unprivileged/lib"
)

var (
	// Upgrader for WebSockets
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true }, // Relax for development
	}
	// Connection to the privileged socket
	privConn net.Conn
	mu       sync.Mutex
)

func main() {
	homeDir, _ := os.UserHomeDir()
	logFilePath := filepath.Join(homeDir, ".ak-demon", "unprivileged", "unprivileged_app.log")
	f, _ := os.Create(logFilePath)

	// Set Gin to log to both the file and standard output
	gin.DefaultWriter = io.MultiWriter(f, os.Stdout)
	log.SetOutput(io.MultiWriter(f, os.Stdout))
	log.SetPrefix("[UNPRIVILEGED] ")
	// log.Println("Worker started and connected to Unix socket")

	log.Printf("Staring...")

	log.Printf("UID: %d, GID: %d", os.Getuid(), os.Getgid())
	log.Printf("Resolved HomeDir: %s", homeDir)
	log.Printf("Target File Path: %s", filepath.Join(homeDir, ".ak-demon", "unprivileged", "pub", "front", "index.html"))

	// 1. Connect to the Privileged socket first
	var err error
	privConn, err = net.Dial("unix", "/var/run/ble_bridge.sock")
	if err != nil {
		log.Fatalf("Failed to connect to privileged socket: %v", err)
	}
	defer privConn.Close()

	// 2. Setup Gin
	r := gin.Default()

	r.GET("/ws", func(c *gin.Context) {
		handleWebSocket(c.Writer, c.Request)
	})

	r.GET("/ok", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{
			"ok": "ok",
		})
	})

	isDev := false

	if len(os.Args) > 1 {
		if os.Args[1] == "dev" {
			isDev = true
		}
	}

	if isDev {
		log.Println("Setting up dev server")
		target, err := url.Parse("http://localhost:3000")

		r.NoRoute(func(ctx *gin.Context) {
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to parse target URL")
				return
			}

			proxy := httputil.NewSingleHostReverseProxy(target)
			proxy.ServeHTTP(ctx.Writer, ctx.Request)
		})
	} else {
		pubDirPath := filepath.Join(homeDir, ".ak-demon", "unprivileged", "pub")
		log.Printf("UID: %d, GID: %d", os.Getuid(), os.Getgid())
		log.Printf("Resolved HomeDir: %s", homeDir)
		log.Printf("Target File Path: %s", filepath.Join(homeDir, ".ak-demon", "unprivileged", "pub", "front", "index.html"))

		r.Static("/pub", pubDirPath)

		r.NoRoute(func(ctx *gin.Context) {

			ctx.File(filepath.Join(pubDirPath, "front", "index.html"))
			// ctx.File("./pub/front/index.html")
		})
	}

	log.Println("Unprivileged Server starting on :8080")
	r.Run(":8080")
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer ws.Close()

	// Listen for messages from SolidJS
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			break
		}

		msg, err, defaulted := lib.ParseFrontendMsgJson(message)

		// log.Printf("%s", string(message))
		// ws.WriteMessage(websocket.TextMessage, []byte("hello"))

		if defaulted {
			log.Printf("Error: No msg type found %s", message)
			continue
		}

		switch m := msg.(type) {
		case lib.TestTypeMsg:
			log.Printf("Test msg from frontend via websocket %s", m.Type)
		case lib.TestMstToPriTypeMsg:
			log.Printf("Test msg from frontend to privileged app %s", m.Type)
			mu.Lock()
			privConn.SetDeadline(time.Now().Add(5 * time.Second))
			privConn.Write(append(message, '\n'))
			resp, _ := bufio.NewReader(privConn).ReadString('\n')
			mu.Unlock()
			ws.WriteMessage(websocket.TextMessage, []byte(resp))
		case lib.TestMstToMobBtTypeMsg:
			log.Printf("Test msg from frontend to mob via BT %s", m.Type)
			mu.Lock()
			privConn.SetDeadline(time.Now().Add(5 * time.Second))
			privConn.Write(append(message, '\n'))
			resp, _ := bufio.NewReader(privConn).ReadString('\n')
			mu.Unlock()
			ws.WriteMessage(websocket.TextMessage, []byte(resp))
		}

		// // Forward message from Frontend to Privileged Process
		// mu.Lock()
		// privConn.Write(append(message, '\n'))

		// // Read response from Privileged Process
		// resp, _ := bufio.NewReader(privConn).ReadString('\n')
		// mu.Unlock()

		// // Send result back to SolidJS
		// ws.WriteMessage(websocket.TextMessage, []byte(resp))
	}
}
