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
	CheckOrigin: func(r *http.Request) bool { return true },
}

// PlayerSession ties a physical WebSocket connection to a SQL user
type PlayerSession struct {
	Conn *websocket.Conn
	User UserRecord
}

var joinQueue = make(chan PlayerSession)

func main() {
	InitDB("AAAdaniel")

	go matchCoordinator()

	http.HandleFunc("/join", handleJoin)

	fmt.Println("🚀 LeetCode Ranked Core Engine listening on http://localhost:8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleJoin(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		username = fmt.Sprintf("Player_%d", time.Now().UnixNano()%1000)
	}

	user, err := GetOrCreateUser(username)
	if err != nil {
		http.Error(w, "Failed to authenticate user", http.StatusInternalServerError)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	joinQueue <- PlayerSession{Conn: conn, User: user}
	select {}
}

type MatchEvent struct {
	Type     string `json:"type"`
	LogMsg   string `json:"log_msg"`
	Template string `json:"template"`
}

func matchCoordinator() {
	var waitingPlayer *PlayerSession = nil
	for {
		session := <-joinQueue

		if waitingPlayer != nil {
			err := waitingPlayer.Conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
			if err != nil {
				waitingPlayer.Conn.Close()
				waitingPlayer = nil
			}
		}

		if waitingPlayer == nil {
			waitingPlayer = &session
			session.Conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Welcome %s (Elo: %d)! In queue...", session.User.Username, session.User.EloRating)))
		} else {
			playerA := *waitingPlayer
			playerB := session
			waitingPlayer = nil

			problem, err := FetchRandomProblem()
			if err != nil {
				log.Println("Error fetching problem:", err)
				continue
			}

			// Create match row in PostgreSQL
			matchID, err := CreateMatchRecord(playerA.User.ID, playerB.User.ID, problem.ID)
			if err != nil {
				log.Println("Error creating match record:", err)
				continue
			}

			var templates map[string]string
			json.Unmarshal([]byte(problem.StarterTemplates), &templates)

			matchMsg := fmt.Sprintf(
				"⚔️ MATCH FOUND!\n%s (%d Elo) vs %s (%d Elo)\nProblem: %s\nDescription: %s\n\nSubmit your solution!",
				playerA.User.Username, playerA.User.EloRating,
				playerB.User.Username, playerB.User.EloRating,
				problem.Title, problem.Description,
			)

			event := MatchEvent{Type: "MATCH_FOUND", LogMsg: matchMsg, Template: templates["python"]}
			jsonBytes, _ := json.Marshal(event)

			playerA.Conn.WriteMessage(websocket.TextMessage, jsonBytes)
			playerB.Conn.WriteMessage(websocket.TextMessage, jsonBytes)

			go handleEvaluationRoom(matchID, playerA, playerB, problem.JudgeConfig)
		}
	}
}

func handleEvaluationRoom(matchID string, playerA PlayerSession, playerB PlayerSession, config ProblemConfig) {
	defer playerA.Conn.Close()
	defer playerB.Conn.Close()

	processSubmission := func(submittingPlayer, opponent PlayerSession, isPlayerA bool) bool {
		_, msg, err := submittingPlayer.Conn.ReadMessage()
		if err != nil {
			opponent.Conn.WriteMessage(websocket.TextMessage, []byte("Opponent disconnected. You win!"))
			return true
		}

		submittedCode := string(msg)
		submittingPlayer.Conn.WriteMessage(websocket.TextMessage, []byte("⏳ Running submission on sandbox..."))
		opponent.Conn.WriteMessage(websocket.TextMessage, []byte("📣 Opponent submitted code! Evaluating..."))

		output, isSuccessful := RunCodeInSandbox(submittedCode, config)

		subStatus := "Wrong Answer"
		passedCases := 0
		if isSuccessful {
			subStatus = "Accepted"
			passedCases = len(config.TestInputs)
		}

		// Record submission in SQL
		RecordSubmission(matchID, submittingPlayer.User.ID, submittedCode, subStatus, passedCases, len(config.TestInputs))

		if isSuccessful {
			winnerNum := 1
			if !isPlayerA {
				winnerNum = 2
			}
			newEloA, newEloB := CalculateElo(playerA.User.EloRating, playerB.User.EloRating, winnerNum)

			if isPlayerA {
				CompleteMatch(matchID, playerA.User.ID, playerB.User.ID, newEloA, newEloB)
			} else {
				CompleteMatch(matchID, playerB.User.ID, playerA.User.ID, newEloB, newEloA)
			}

			winMsgA := fmt.Sprintf("\n✅ SUCCESS:\n%s\n🏆 YOU WIN! New Elo: %d", output, newEloA)
			loseMsgB := fmt.Sprintf("\n🚨 OPPONENT PASSED:\n%s\n💀 MATCH OVER. New Elo: %d", output, newEloB)

			if !isPlayerA {
				winMsgA = fmt.Sprintf("\n🚨 OPPONENT PASSED:\n%s\n💀 MATCH OVER. New Elo: %d", output, newEloA)
				loseMsgB = fmt.Sprintf("\n✅ SUCCESS:\n%s\n🏆 YOU WIN! New Elo: %d", output, newEloB)
			}

			playerA.Conn.WriteMessage(websocket.TextMessage, []byte(winMsgA))
			playerB.Conn.WriteMessage(websocket.TextMessage, []byte(loseMsgB))

			return true
		} else {
			submittingPlayer.Conn.WriteMessage(websocket.TextMessage, []byte("\n❌ FAILURE:\n"+output))
			opponent.Conn.WriteMessage(websocket.TextMessage, []byte("\n📣 Opponent's submission failed."))
			return false
		}
	}

	go func() {
		for {
			if processSubmission(playerA, playerB, true) {
				playerA.Conn.Close()
				playerB.Conn.Close()
				return
			}
		}
	}()

	for {
		if processSubmission(playerB, playerA, false) {
			playerA.Conn.Close()
			playerB.Conn.Close()
			return
		}
	}
}
