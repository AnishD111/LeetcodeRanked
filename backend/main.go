package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// ─────────────────────────────────────────────────────────────────────────────
// WEBSOCKET UPGRADER
//
// HTTP and WebSocket are two different protocols, but a WebSocket connection
// always starts as an HTTP request. The "upgrade" is when our server agrees
// to switch the connection from HTTP to the persistent WebSocket protocol.
//
// CheckOrigin: We return true for all origins during development.
// In production, you'd validate r.Header.Get("Origin") against an allowlist
// of your frontend domains to prevent unauthorized sites from opening sockets.
// ─────────────────────────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Lock this down to Cfg.FrontendURL in production.
		return true
	},
}

// PlayerSession ties a physical WebSocket connection to a verified SQL user.
// This is populated AFTER authentication succeeds in handleJoin.
type PlayerSession struct {
	Conn *websocket.Conn
	User UserRecord
}

// joinQueue is a channel that the matchCoordinator goroutine reads from.
// When a player connects and authenticates via WebSocket, their PlayerSession
// is sent into this channel. The coordinator pairs them up.
//
// WHAT IS A CHANNEL?
// A Go channel is a typed pipe for communication between goroutines.
// Sending to a channel blocks until something receives from it, and vice versa.
// This is Go's built-in mechanism for safe concurrent communication — no mutexes needed.
var joinQueue = make(chan PlayerSession)

// MatchEvent is the structured JSON event sent to both players when a match starts.
type MatchEvent struct {
	Type        string `json:"type"`
	LogMsg      string `json:"log_msg"`
	Template    string `json:"template"`
	ProblemID   string `json:"problem_id"`
	ProblemName string `json:"problem_name"`
	Description string `json:"description"`
}

// ─────────────────────────────────────────────────────────────────────────────
// MAIN — Application Entry Point
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	// 1. Load environment variables from .env file.
	LoadConfig()

	// 2. Initialize the database pool and run migrations.
	InitDB()

	// 3. Start the matchmaking coordinator in a background goroutine.
	// The `go` keyword launches matchCoordinator() concurrently — it runs
	// in its own goroutine and blocks on the joinQueue channel, waiting for
	// players to pair. The main goroutine continues to start the HTTP server.
	go matchCoordinator()

	// ── Route Registration ────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Auth REST endpoints — no auth required (public)
	mux.HandleFunc("/api/auth/register", handleRegister)
	mux.HandleFunc("/api/auth/login", handleLogin)
	mux.HandleFunc("/api/auth/logout", handleLogout)

	// OAuth 2.0 flow endpoints — no auth required (they establish it)
	mux.HandleFunc("/api/auth/google", handleGoogleLogin)
	mux.HandleFunc("/api/auth/google/callback", handleGoogleCallback)
	mux.HandleFunc("/api/auth/github", handleGitHubLogin)
	mux.HandleFunc("/api/auth/github/callback", handleGitHubCallback)

	// Protected endpoints — require valid JWT cookie via RequireAuth middleware
	mux.Handle("/api/auth/me", RequireAuth(http.HandlerFunc(handleMe)))
	mux.Handle("/api/user/elo-history", RequireAuth(http.HandlerFunc(handleGetEloHistory)))


	// WebSocket endpoint — auth is enforced inside handleJoin before upgrading
	mux.HandleFunc("/ws/join", handleJoin)

	// 4. Apply CORS middleware to the entire mux.
	handler := corsMiddleware(mux)

	addr := ":" + Cfg.Port
	fmt.Printf("🚀 LeetCode Ranked Backend listening on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

// ─────────────────────────────────────────────────────────────────────────────
// CORS MIDDLEWARE
//
// WHAT IS CORS?
// Browsers enforce the "Same-Origin Policy": JavaScript on page A cannot make
// HTTP requests to page B unless B explicitly allows it via CORS headers.
//
// When our frontend (localhost:3000) makes a fetch() to our backend (localhost:8080),
// the browser performs a "preflight" OPTIONS request first, asking:
//   "Does this server allow cross-origin requests from my origin?"
//
// If we don't respond with the right Access-Control-Allow-* headers, the
// browser blocks the request before it reaches our handler.
//
// NOTE: credentials: true (for cookies) requires Access-Control-Allow-Origin
// to be a specific origin, NOT the wildcard "*". We use Cfg.FrontendURL.
// ─────────────────────────────────────────────────────────────────────────────

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow the frontend origin specifically.
		w.Header().Set("Access-Control-Allow-Origin", Cfg.FrontendURL)
		// Allow cookies/credentials to be included in cross-origin requests.
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		// Respond to preflight requests immediately.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// WEBSOCKET JOIN HANDLER
// GET /ws/join  (HTTP → WebSocket Upgrade)
// ─────────────────────────────────────────────────────────────────────────────

// handleJoin is the entry point for players connecting to the matchmaking queue.
// It MUST authenticate the user before upgrading — see middleware.go for why.
func handleJoin(w http.ResponseWriter, r *http.Request) {
	// ── Authentication (BEFORE upgrade) ───────────────────────────────────────
	// authenticateWSRequest reads the auth_token cookie, validates the JWT,
	// and fetches the user from DB. All of this happens on the HTTP connection,
	// before we agree to upgrade to WebSocket.
	user, err := authenticateWSRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return // We reject here — no WebSocket upgrade occurs for unauthenticated users.
	}

	// ── WebSocket Upgrade ──────────────────────────────────────────────────────
	// Now that we've verified the user, we upgrade the HTTP connection to WebSocket.
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	// ── Queue the player ───────────────────────────────────────────────────────
	session := PlayerSession{Conn: conn, User: user}

	// Send the session to the matchCoordinator goroutine via the channel.
	// This line BLOCKS (the goroutine running handleJoin waits here) until
	// the coordinator reads from the channel. That's fine — each WebSocket
	// connection runs its own goroutine.
	joinQueue <- session

	// After queuing, we block forever with an empty select{} to keep this
	// goroutine (and the WebSocket connection) alive. The coordinator and
	// evaluation room goroutines manage the connection from here.
	select {}
}

// ─────────────────────────────────────────────────────────────────────────────
// MATCHMAKING COORDINATOR
// Runs as a single goroutine for the lifetime of the server.
// ─────────────────────────────────────────────────────────────────────────────

// matchCoordinator is a never-ending loop that pairs incoming PlayerSessions.
// It maintains a single `waitingPlayer` slot. When that slot is empty, it stores
// the new arrival and sends them a "waiting" message. When the slot is filled
// and a new player arrives, it pairs them and launches an evaluation room.
func matchCoordinator() {
	var waitingPlayer *PlayerSession = nil

	for {
		session := <-joinQueue // Blocks until a player joins

		// Liveness check: ping the waiting player to see if they're still connected.
		// Players might disconnect while in queue (browser closed, network dropped).
		if waitingPlayer != nil {
			err := waitingPlayer.Conn.WriteControl(
				websocket.PingMessage, []byte{}, time.Now().Add(time.Second),
			)
			if err != nil {
				// Waiting player disconnected — clear the slot.
				waitingPlayer.Conn.Close()
				waitingPlayer = nil
			}
		}

		if waitingPlayer == nil {
			// No opponent yet — hold this player in the waiting slot.
			waitingPlayer = &session
			session.Conn.WriteMessage(websocket.TextMessage, []byte(
				fmt.Sprintf("⏳ Welcome %s (Elo: %d)! Waiting for an opponent...", session.User.Username, session.User.EloRating),
			))
		} else {
			// We have two players — start a match!
			playerA := *waitingPlayer
			playerB := session
			waitingPlayer = nil // Reset the slot immediately

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
				"⚔️ MATCH FOUND!\n%s (%d Elo) vs %s (%d Elo)\nProblem: %s\n%s",
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

			// Launch the evaluation room in its own goroutine so the coordinator
			// loop immediately returns to wait for the next pair of players.
			go handleEvaluationRoom(matchID, playerA, playerB, problem.JudgeConfig)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EVALUATION ROOM
// Manages concurrent submission reads for both players.
// ─────────────────────────────────────────────────────────────────────────────

// handleEvaluationRoom manages the active match between two connected players.
// It runs concurrently — each active match has its own goroutine (plus one sub-goroutine
// per player for submission reads). When either player passes all tests, the match
// ends, Elo is updated, and both WebSocket connections are closed.
func handleEvaluationRoom(matchID string, playerA PlayerSession, playerB PlayerSession, config JudgeConfig) {
	defer playerA.Conn.Close()
	defer playerB.Conn.Close()

	// processSubmission reads one message from the submitting player's WebSocket,
	// runs it through the sandbox, records the result, and broadcasts outcomes.
	// Returns true if the match should end (success OR opponent disconnected).
	processSubmission := func(submitter, opponent PlayerSession) bool {
		_, msg, err := submitter.Conn.ReadMessage()
		if err != nil {
			opponent.Conn.WriteMessage(websocket.TextMessage, []byte("🏆 Opponent disconnected. You win by default!"))
			return true
		}

		userCode := string(msg)
		submitter.Conn.WriteMessage(websocket.TextMessage, []byte("⏳ Running your submission in sandbox..."))
		opponent.Conn.WriteMessage(websocket.TextMessage, []byte("📣 Your opponent submitted! Evaluating..."))

		output, passed := RunCodeInSandbox(userCode, config)

		RecordSubmission(matchID, submitter.User.ID, userCode, passed)

		if passed {
			// Determine winner/loser for Elo and DB update
			winnerID := submitter.User.ID
			loserID := opponent.User.ID

			wOld, wNew, lOld, lNew, err := CompleteMatch(matchID, winnerID, loserID)
			if err != nil {
				log.Println("CompleteMatch error:", err)
			}

			wDelta := wNew - wOld
			lDelta := lNew - lOld

			submitter.Conn.WriteMessage(websocket.TextMessage, []byte(
				fmt.Sprintf("\n✅ SUCCESS:\n%s\n🏆 YOU WIN!\nRating: %d -> %d (+%d)", output, wOld, wNew, wDelta),
			))
			opponent.Conn.WriteMessage(websocket.TextMessage, []byte(
				fmt.Sprintf("\n🚨 OPPONENT PASSED:\n%s\n💀 MATCH OVER.\nRating: %d -> %d (%d)", output, lOld, lNew, lDelta),
			))
			return true
		}


		submitter.Conn.WriteMessage(websocket.TextMessage, []byte("\n❌ FAILURE:\n"+output))
		opponent.Conn.WriteMessage(websocket.TextMessage, []byte("\n📣 Opponent's submission failed. Keep going!"))
		return false
	}

	// Run each player's submission loop in concurrent goroutines.
	// IMPORTANT: Both goroutines share the same deferred Conn.Close() calls above.
	// When either goroutine closes a connection, ReadMessage() on the other will
	// return an error, which causes that goroutine to exit cleanly too.
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
