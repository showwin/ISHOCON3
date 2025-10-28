package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		s.log.Error("Failed to parse departure time", "error", err.Error())
		return err
	}
	currentTime, err := time.ParseInLocation("15:04", currentTimeStr, jst)
	if err != nil {
		s.log.Error("Failed to parse current time", "error", err.Error())
		return err
	}
	waitTime := max(departureTime.Add(-1*time.Hour).Sub(currentTime), 0)
	s.log.Info("Thinking about whether to enter", "departureAt", departureAt, "current_time", currentTimeStr, "entryToken", entryToken, "user", user.Name)

	if waitTime > 0 {
		waitTimeInApp := waitTime / 600 // 1 second in app time is 10 minutes in real time
		s.log.Info("Waiting until 1 hour before departure", "wait_time", waitTime.String(), "wait_time_in_app", waitTimeInApp.String(), "departure_time", departureAt, "current_time", currentTimeStr, "user", user.Name)
		time.Sleep(waitTimeInApp)
	} else {
		// Moving to the gate takes 10 minutes in real time (1 second in app time)
		time.Sleep(1 * time.Second)
	}

	currentTimeStr = getApplicationClock(s.initializedAt)
	s.log.Info("Arrived at ticket gate", "departureAt", departureAt, "current_time", currentTimeStr, "entryToken", entryToken, "user", user.Name)

	// Enter the ticket gate
	resp, err := s.enterGate(ctx, EntryReq{EntryToken: entryToken}, user)
	if err != nil {
		s.log.Error("Failed to enter", "error", err.Error(), "token", entryToken)
	}
	currentTimeStr = getApplicationClock(s.initializedAt)

	if resp.Status == "train_departed" {
		s.log.Info("Train has already departed. The ticket was too close to departure time.", "token", entryToken, "departure_time", departureAt, "current_time", currentTimeStr, "user", user.Name)
		s.log.Info("Logging in again to refund", "token", entryToken, "user", user.Name)
		err := s.runRefundScenario(ctx, user, reservation.ReservationID, reservation.TotalPrice)
		if err != nil {
			s.log.Error("Failed to refund", "error", err.Error(), "user", user.Name)
			// TODO: forcibly stop benchmark
		}
		return nil
	}
	s.log.Info("Entered the ticket gate", "departure_time", departureAt, "current_time", currentTimeStr, "token", entryToken, "from", reservation.FromStation, "to", reservation.ToStation, "user", user.Name)
	// Add sales
	s.totalSales.Add(int64(reservation.TotalPrice))
	s.log.Info("Sales recorded", "amount", reservation.TotalPrice, "user", user.Name)

	return nil
}

func (s *Scenario) enterGate(ctx context.Context, req EntryReq, user User) (*EntryResp, error) {
	agent, err := agent.NewAgent(agent.WithBaseURL(s.targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
	if err != nil {
		s.log.Error("Failed to create agent", err.Error())
	}

	reqBodyBuf, err := json.Marshal(req)
	if err != nil {
		s.log.Error("Failed to parse JSON", "error", err.Error(), "token", req.EntryToken, "user", user.Name)
		return nil, err
	}
	resp, err := HttpPost(ctx, agent, "/api/entry", bytes.NewReader(reqBodyBuf))
	if err != nil {
		s.log.Error("Failed to post /api/entry", "error", err.Error(), "token", req.EntryToken, "user", user.Name)
		return nil, err
	}
	s.log.Info("POST /api/entry", "statusCode", resp.StatusCode, "token", req.EntryToken, "user", user.Name)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("got %d status code from /api/entry", resp.StatusCode)
	}

	var entryResp EntryResp
	if err := json.Unmarshal(resp.Body, &entryResp); err != nil {
		s.log.Error("Failed to unmarshal response", "error", err.Error(), "token", req.EntryToken, "user", user.Name)
		return nil, err
	}

	return &entryResp, nil
}

func (s *Scenario) runRefundScenario(ctx context.Context, user User, reservationID string, totalPrice int) error {
	agent, err := agent.NewAgent(agent.WithBaseURL(s.targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
	if err != nil {
		s.log.Error("Failed to create agent", err.Error())
	}

	s.postLogin(ctx, agent, user)

	s.waitInWaitingRoom(ctx, agent, user)

	childCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker to periodically GET `/api/schedules`
	scheduleWorker, err := worker.NewWorker(func(childCtx context.Context, _ int) {
		resp, err := HttpGet(childCtx, agent, "/api/schedules")
		if err != nil {
			s.log.Error("Failed to get /api/schedules", "error", err.Error(), "user", user.Name)
		}
		s.log.Debug("GET /api/schedules", "statusCode", resp.StatusCode, "user", user.Name)
		time.Sleep(1 * time.Second)
	}, worker.WithInfinityLoop(), worker.WithMaxParallelism(1))
	if err != nil {
		s.log.Error("Failed to create GET /api/schedule worker", "error", err.Error(), "user", user.Name)
	}
	go func() {
		scheduleWorker.Process(childCtx)
	}()

	// Request refund
	refundResp, err := s.requestRefund(ctx, agent, user, reservationID)
	if err != nil {
		s.log.Error("Failed to request refund", err.Error(), "user", user.Name)
		return err
	}

	// Add refund amount if successful
	if refundResp.Status == "success" {
		s.totalRefunds.Add(int64(totalPrice))
		s.log.Info("Refund recorded", "amount", totalPrice, "user", user.Name)
	}
	s.log.Info("Refund request succeeded", "user", user.Name)

	// Finish if the session is expired
	s.checkSession(ctx, agent, user)

	s.log.Info("Session ended", "user", user.Name)
	return nil
}

func (s *Scenario) requestRefund(ctx context.Context, agent *agent.Agent, user User, reservationID string) (*RefundResp, error) {
	reqBodyBuf, err := json.Marshal(RefundReq{ReservationID: reservationID})
	if err != nil {
		s.log.Error("Failed to parse JSON", "error", err.Error(), "user", user.Name)
		return nil, err
	}
	resp, err := HttpPost(ctx, agent, "/api/refund", bytes.NewReader(reqBodyBuf))
	if err != nil {
		s.log.Error("Failed to post /api/refund", "error", err.Error(), "user", user.Name)
		return nil, err
	}
	s.log.Info("POST /api/refund", "statusCode", resp.StatusCode, "user", user.Name)

	var refundResp RefundResp
	if err := json.Unmarshal(resp.Body, &refundResp); err != nil {
		s.log.Error("Failed to unmarshal response", "error", err.Error(), "user", user.Name)
		return nil, err
	}

	return &refundResp, nil
}
