let socket = null;

// Select DOM elements once at startup
const statusBadge = document.getElementById('status');
const codeEditor = document.getElementById('codeEditor');
const submitBtn = document.getElementById('submitBtn');
const connectBtn = document.getElementById('connectBtn');
const logStream = document.getElementById('logStream');

function appendLog(text, type = 'system') {
    const entry = document.createElement('div');
    entry.className = `log-entry log-${type}`;
    entry.innerText = `[${new Date().toLocaleTimeString()}] ${text}`;
    logStream.appendChild(entry);
    logStream.scrollTop = logStream.scrollHeight; // Keep view pinned to bottom
}

function connectToServer() {
    const username = document.getElementById('usernameInput').value.trim() || 'Anonymous';
    appendLog(`Connecting as ${username}...`, "system");
    connectBtn.disabled = true;

    // Connect directly to our running Go backend
    socket = new WebSocket(`ws://localhost:8080/join?username=${encodeURIComponent(username)}`);
    socket.onopen = () => {
        statusBadge.innerText = "Queued";
        statusBadge.className = "status-badge connected";
        appendLog("Connected to server! Placed into the matchmaking line...", "system");
    };

    socket.onmessage = (event) => {
        const rawData = event.data;

        try {
            // 1. Attempt to parse as JSON (Matches our new MatchEvent struct)
            const data = JSON.parse(rawData);

            if (data.type === "MATCH_FOUND") {
                statusBadge.innerText = "In Match";

                // Unlock the UI
                codeEditor.disabled = false;
                submitBtn.disabled = false;

                // Inject the Python starter template!
                codeEditor.value = data.template;

                // Print the rich problem description
                appendLog(data.log_msg, "system");
            }
        } catch (e) {
            // 2. Fallback: If it's not JSON, it's a raw text log from Go or the Sandbox
            const message = rawData;

            if (message.includes("SUCCESS:")) {
                appendLog(message, "success");
            } else if (message.includes("FAILURE:") || message.includes("TIMEOUT")) {
                appendLog(message, "failure");
            } else {
                // Catch-all for basic system messages like "You are in the queue..."
                appendLog(message, "system");
            }
        }
    };

    socket.onclose = () => {
        statusBadge.innerText = "Disconnected";
        statusBadge.className = "status-badge";
        codeEditor.disabled = true;
        submitBtn.disabled = true;
        connectBtn.disabled = false;
        appendLog("Connection dropped or closed.", "failure");
    };

    socket.onerror = (error) => {
        appendLog("WebSocket interface error encountered.", "failure");
        console.error(error);
    };
}

function submitCode() {
    const code = codeEditor.value;
    if (!code.trim()) return;

    appendLog("Sending submission to backend...", "system");
    socket.send(code);
}