package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Cogria-AI/cogriaclaw/internal/skills"
	"github.com/Cogria-AI/cogriaclaw/internal/wa"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	self := s.deps.WA.Self()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"connected": s.deps.WA.IsConnected(),
		"self":      self.User,
		"uptime_s":  int(time.Since(s.deps.Started).Seconds()),
	})
}

type sendRequest struct {
	To   string `json:"to"` // E.164 (+447...) or group JID (...@g.us)
	Text string `json:"text"`
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, errBody("text is required"))
		return
	}
	to, err := wa.ParseTarget(req.To)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid 'to': "+err.Error()))
		return
	}
	if err := s.deps.WA.SendText(r.Context(), to, req.Text); err != nil {
		slog.Error("api: send failed", "err", err, "to", to.String())
		writeJSON(w, http.StatusBadGateway, errBody("send failed: "+err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "to": to.String()})
}

type triggerRequest struct {
	Skill  string          `json:"skill"`
	Input  json.RawMessage `json:"input"`
	Notify *struct {
		To string `json:"to"`
	} `json:"notify"`
}

func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	var req triggerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	if req.Skill == "" {
		writeJSON(w, http.StatusBadRequest, errBody("skill is required"))
		return
	}
	sk, ok := s.deps.Skills.Get(req.Skill)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("unknown skill: "+req.Skill))
		return
	}
	input := req.Input
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}

	// API-triggered run: no inbound message context.
	sc := &skills.Ctx{WA: s.deps.WA, Inbound: nil}
	result, err := sk.Run(r.Context(), sc, input)
	if err != nil {
		slog.Error("api: skill run failed", "skill", req.Skill, "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody("skill failed: "+err.Error()))
		return
	}

	resp := map[string]any{"ok": true, "result": result}
	if req.Notify != nil && req.Notify.To != "" {
		to, err := wa.ParseTarget(req.Notify.To)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errBody("invalid 'notify.to': "+err.Error()))
			return
		}
		if err := s.deps.WA.SendText(r.Context(), to, result); err != nil {
			slog.Error("api: notify send failed", "err", err, "to", to.String())
			resp["notified"] = false
			resp["notify_error"] = err.Error()
		} else {
			resp["notified"] = true
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)) // 1 MiB cap
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("invalid JSON body: " + err.Error())
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]any {
	return map[string]any{"ok": false, "error": msg}
}
