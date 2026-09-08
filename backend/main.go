package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type PlayerSession struct {
	Conn *websocket.Conn
	User UserRecord
}

var joinQueue = make(chan PlayerSession)

type MatchEvent struct {
	Type        string `json:"type"`
	LogMsg      string `json:"log_msg"`
	Template    string `json:"template"`
	ProblemID   string `json:"problem_id"`
	ProblemName string `json:"problem_name"`
	Description string `json:"description"`
}

func main() {
	LoadConfig()
	InitOAuth()
	InitDB()

	go matchCoordinator()

	mux := http.NewServeMux()

	// Auth REST endpoints
	mux.HandleFunc("/api/auth/register", handleRegister)
	mux.HandleFunc("/api/auth/login", handleLogin)
	mux.HandleFunc("/api/auth/logout", handleLogout)

	// OAuth 2.0 flow endpoints
	mux.HandleFunc("/api/auth/google", handleGoogleLogin)
	mux.HandleFunc("/api/auth/google/callback", handleGoogleCallback)
	mux.HandleFunc("/api/auth/github", handleGitHubLogin)
	mux.HandleFunc("/api/auth/github/callback", handleGitHubCallback)

	// Protected endpoints
	mux.Handle("/api/auth/me", RequireAuth(http.HandlerFunc(handleMe)))
	mux.Handle("/api/user/elo-history", RequireAuth(http.HandlerFunc(handleGetEloHistory)))

	// WebSocket endpoint
	mux.HandleFunc("/ws/join", handleJoin)

	handler := corsMiddleware(mux)

	addr := ":" + Cfg.Port
	fmt.Printf("LeetCode Ranked Backend listening on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", Cfg.FrontendURL)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func handleJoin(w http.ResponseWriter, r *http.Request) {
	user, err := authenticateWSRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	session := PlayerSession{Conn: conn, User: user}
	joinQueue <- session

	select {}
}

func matchCoordinator() {
	var waitingPlayer *PlayerSession = nil

	for {
		session := <-joinQueue

		if waitingPlayer != nil {
			err := waitingPlayer.Conn.WriteControl(
				websocket.PingMessage, []byte{}, time.Now().Add(time.Second),
			)
			if err != nil {
				waitingPlayer.Conn.Close()
				waitingPlayer = nil
			}
		}

		if waitingPlayer == nil {
			waitingPlayer = &session
			session.Conn.WriteMessage(websocket.TextMessage, []byte(
				fmt.Sprintf("Welcome %s (Elo: %d)! Waiting for an opponent...", session.User.Username, session.User.EloRating),
			))
		} else {
			playerA := *waitingPlayer
			playerB := session
			waitingPlayer = nil

			problem, err := FetchRandomProblem()
			if err != nil {
				log.Println("Error fetching problem:", err)
				continue
			}

			matchID, err := CreateMatchRecord(playerA.User.ID, playerB.User.ID, problem.ID)
			if err != nil {
				log.Println("Error creating match record:", err)
				continue
			}

			matchMsg := fmt.Sprintf(
				"MATCH FOUND!\n%s (%d Elo) vs %s (%d Elo)\nProblem: %s\n%s",
				playerA.User.Username, playerA.User.EloRating,
				playerB.User.Username, playerB.User.EloRating,
				problem.Title, problem.Description,
			)

			event := MatchEvent{
				Type:        "MATCH_FOUND",
				LogMsg:      matchMsg,
				Template:    problem.StarterTemplates["python"],
				ProblemID:   problem.ID,
				ProblemName: problem.Title,
				Description: problem.Description,
			}
			jsonBytes, _ := json.Marshal(event)

			playerA.Conn.WriteMessage(websocket.TextMessage, jsonBytes)
			playerB.Conn.WriteMessage(websocket.TextMessage, jsonBytes)

			go handleEvaluationRoom(matchID, playerA, playerB, problem.JudgeConfig)
		}
	}
}

func handleEvaluationRoom(matchID string, playerA PlayerSession, playerB PlayerSession, config JudgeConfig) {
	defer playerA.Conn.Close()
	defer playerB.Conn.Close()

	processSubmission := func(submitter, opponent PlayerSession) bool {
		_, msg, err := submitter.Conn.ReadMessage()
		if err != nil {
			opponent.Conn.WriteMessage(websocket.TextMessage, []byte("Opponent disconnected. You win by default!"))
			return true
		}

		userCode := string(msg)
		submitter.Conn.WriteMessage(websocket.TextMessage, []byte("Running your submission in sandbox..."))
		opponent.Conn.WriteMessage(websocket.TextMessage, []byte("Your opponent submitted! Evaluating..."))

		output, passed := RunCodeInSandbox(userCode, config)
		RecordSubmission(matchID, submitter.User.ID, userCode, passed)

		if passed {
			winnerID := submitter.User.ID
			loserID := opponent.User.ID

			wOld, wNew, lOld, lNew, err := CompleteMatch(matchID, winnerID, loserID)
			if err != nil {
				log.Println("CompleteMatch error:", err)
			}

			wDelta := wNew - wOld
			lDelta := lNew - lOld

			submitter.Conn.WriteMessage(websocket.TextMessage, []byte(
				fmt.Sprintf("\nSUCCESS:\n%s\nYOU WIN!\nRating: %d -> %d (+%d)", output, wOld, wNew, wDelta),
			))
			opponent.Conn.WriteMessage(websocket.TextMessage, []byte(
				fmt.Sprintf("\nOPPONENT PASSED:\n%s\nMATCH OVER.\nRating: %d -> %d (%d)", output, lOld, lNew, lDelta),
			))
			return true
		}

		submitter.Conn.WriteMessage(websocket.TextMessage, []byte("\nFAILURE:\n"+output))
		opponent.Conn.WriteMessage(websocket.TextMessage, []byte("\nOpponent's submission failed. Keep going!"))
		return false
	}

	done := make(chan struct{})

	go func() {
		for {
			if processSubmission(playerA, playerB) {
				close(done)
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
			if processSubmission(playerB, playerA) {
				return
			}
		}
	}
}
