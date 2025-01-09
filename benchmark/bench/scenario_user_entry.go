package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucandar/worker"
)

type EntryReq struct {
	EntryToken string `json:"entry_token"`
}

type EntryResp struct {
	Status string `json:"status"`
}

type RefundReq struct {
	ReservationID string `json:"reservation_id"`
}

type RefundResp struct {
	Status string `json:"status"`
}

func (s *Scenario) runEntryScenario(ctx context.Context, user User, reservation Reservation, entryToken string) error {
	currentTimeStr := getApplicationClock(s.initializedAt)
	departureAt := reservation.DepartureAt

	// Wait until 1 hour before the departure time
	departureTime, err := time.ParseInLocation("15:04", departureAt, jst)
	if err != nil {
		s.log.Error("failed to parse departure time", "error", err.Error())
		return err
	}
	currentTime, _ := time.ParseInLocation("15:04", currentTimeStr, jst)
	waitTime := departureTime.Add(-1 * time.Hour).Sub(currentTime)
	if waitTime > 0 {
		waitTimeInApp := waitTime / 600 // 1 second in app time is 10 minutes in real time
		s.log.Info("waiting until 1 hour before departure", "wait_time", waitTime.String(), "departure_time", departureAt, "current_time", currentTimeStr)
		time.Sleep(waitTimeInApp)
	}

	// Entry
	resp, err := s.entry(ctx, EntryReq{EntryToken: entryToken}, user)
	if err != nil {
		s.log.Error("failed to entry", "error", err.Error(), "token", entryToken)
	}

	if resp.Status == "train_departed" {
		currentTimeStr = getApplicationClock(s.initializedAt)
		s.log.Info("train has already departed", "token", entryToken, "departure_time", departureAt, "current_time", currentTimeStr)
		s.runRefundScenario(ctx, user, reservation.ReservationID)
	}

	return nil
}

func (s *Scenario) entry(ctx context.Context, req EntryReq, user User) (*EntryResp, error) {
	agent, err := agent.NewAgent(agent.WithBaseURL(s.targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
	if err != nil {
		s.log.Error("failed to create agent", err.Error())
	}

	reqBodyBuf, err := json.Marshal(req)
	if err != nil {
		s.log.Error("failed to parse JSON", "error", err.Error(), "token", req.EntryToken, "user", user.Name)
		return nil, err
	}
	resp, err := HttpPost(ctx, agent, "/api/entry", bytes.NewReader(reqBodyBuf))
	if err != nil {
		s.log.Error("failed to post /api/entry", "error", err.Error(), "token", req.EntryToken, "user", user.Name)
		return nil, err
	}
	s.log.Info("POST /api/entry", "statusCode", resp.StatusCode, "token", req.EntryToken, "user", user.Name)

	var entryResp EntryResp
	if err := json.Unmarshal(resp.Body, &entryResp); err != nil {
		s.log.Error("failed to unmarshal response", "error", err.Error(), "token", req.EntryToken, "user", user.Name)
		return nil, err
	}

	return &entryResp, nil
}

func (s *Scenario) runRefundScenario(ctx context.Context, user User, reservationID string) error {
	agent, err := agent.NewAgent(agent.WithBaseURL(s.targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
	if err != nil {
		s.log.Error("failed to create agent", err.Error())
	}

	s.postLogin(ctx, agent, user)

	s.waitInWaitingRoom(ctx, agent, user)

	childCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker to periodically GET `/api/schedules`
	scheduleWorker, err := worker.NewWorker(func(childCtx context.Context, _ int) {
		resp, err := HttpGet(childCtx, agent, "/api/schedules")
		if err != nil {
			s.log.Error("failed to get /api/schedules", "error", err.Error(), "user", user.Name)
		}
		s.log.Debug("GET /api/schedules", "statusCode", resp.StatusCode, "user", user.Name)
		time.Sleep(1 * time.Second)
	}, worker.WithInfinityLoop(), worker.WithMaxParallelism(1))
	if err != nil {
		s.log.Error("failed to create GET /api/schedule worker", "error", err.Error(), "user", user.Name)
	}
	go func() {
		scheduleWorker.Process(childCtx)
	}()

	// Request refund
	_, err = s.requestRefund(ctx, agent, user, reservationID)
	if err != nil {
		s.log.Error("failed to request refund", err.Error(), "user", user.Name)
	}

	// Finish if the session is expired
	s.checkSession(ctx, agent, user)

	s.log.Info("user", user.Name, "Session ended", "user", user.Name)
	return nil
}

func (s *Scenario) requestRefund(ctx context.Context, agent *agent.Agent, user User, reservationID string) (*RefundResp, error) {
	reqBodyBuf, err := json.Marshal(RefundReq{ReservationID: reservationID})
	if err != nil {
		s.log.Error("failed to parse JSON", "error", err.Error(), "user", user.Name)
		return nil, err
	}
	resp, err := HttpPost(ctx, agent, "/api/refund", bytes.NewReader(reqBodyBuf))
	if err != nil {
		s.log.Error("failed to post /api/refund", "error", err.Error(), "user", user.Name)
		return nil, err
	}
	s.log.Info("POST /api/refund", "statusCode", resp.StatusCode, "user", user.Name)

	var refundResp RefundResp
	if err := json.Unmarshal(resp.Body, &refundResp); err != nil {
		s.log.Error("failed to unmarshal response", "error", err.Error(), "user", user.Name)
		return nil, err
	}

	return &refundResp, nil
}
