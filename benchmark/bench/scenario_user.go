package bench

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"bytes"
	"time"
	"context"
	"os"
	"math/rand"
	"encoding/json"

	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucandar/worker"
)


type User struct {
    Name               string
    Password           string
    GlobalPaymentToken string
}

type TrainAvailability struct {
	ArenaToBridge string `json:"Arena->Bridge"`
  BridgeToCave  string `json:"Bridge->Cave"`
	CaveToDock string `json:"Cave->Dock"`
	DockToEdge string `json:"Dock->Edge"`
	EdgeToDock string `json:"Edge->Dock"`
	DockToCave string `json:"Dock->Cave"`
	CaveToBridge string `json:"Cave->Bridge"`
	BridgeToArena string `json:"Bridge->Arena"`
}

type TrainDepartureAt struct {
	ArenaToBridge string `json:"Arena->Bridge"`
	BridgeToCave  string `json:"Bridge->Cave"`
	CaveToDock string `json:"Cave->Dock"`
	DockToEdge string `json:"Dock->Edge"`
	EdgeToDock string `json:"Edge->Dock"`
	DockToCave string `json:"Dock->Cave"`
	CaveToBridge string `json:"Cave->Bridge"`
	BridgeToArena string `json:"Bridge->Arena"`
}

type TrainSchedule struct {
	ID          string `json:"id"`
	Availability TrainAvailability `json:"availability"`
	departureAt TrainDepartureAt `json:"departure_at"`
}

type TrainScheduleResp struct {
	Schedules []TrainSchedule  `json:"schedules"`
}

// type BoughtTicket struct {
//   entryToken string
//   departureAt string
// }

type LoginReq struct {
	Name string `json:"name"`
	Password string `json:"password"`
}

type WaitingStatusResp struct {
	Status		string `json:"status"`
	NextCheck int    `json:"next_check"`
}

type SessionResp struct {
  Status    string `json:"status"`
  NextCheck int    `json:"next_check"`
}

func (s *Scenario) RunUserScenario(ctx context.Context) {
	targetURL := ctx.Value(targetURLKey).(string)
	agent, err := agent.NewAgent(agent.WithBaseURL(targetURL), agent.WithDefaultTransport())
	if err != nil {
		s.log.Error("failed to create agent", err.Error())
	}

	user, err := s.getRandomUser()
	if err != nil {
		s.log.Error("failed to get random user", err.Error())
	}
	s.log.Info("START", "user", user.Name)

	s.log.Debug("POST /api/login", "user", user.Name)
	reqBody := &LoginReq{
		Name: user.Name,
		Password: user.Password,
	}
	reqBodyBuf, err := json.Marshal(reqBody)
	if err != nil {
			s.log.Error("failed to parse JSON", err.Error(), "user", user.Name)
			return
	}
	resp, err := HttpPost(ctx, agent, "/api/login", bytes.NewReader(reqBodyBuf))
	if err != nil {
		s.log.Error("failed to post /api/login", err.Error(), "user", user.Name)
	}
	s.log.Debug("POST /api/login", "statusCode", resp.StatusCode, "user", user.Name)

	s.waitInWaitingRoom(ctx, agent, user)

	childCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.log.Info("Schedule worker preparing", "user", user.Name)
	// Start worker to infinitely GET /api/schedules
	scheduleWorker, err := worker.NewWorker(func(childCtx context.Context, _ int) {
		resp, err := HttpGet(childCtx, agent, "/api/schedules")
		if err != nil {
			s.log.Error("failed to get /api/schedules", err.Error(), "user", user.Name)
		}
		s.log.Info("GET /api/schedules", "statusCode", resp.StatusCode, "user", user.Name)
		time.Sleep(1 * time.Second)
	}, worker.WithInfinityLoop(), worker.WithMaxParallelism(1))
	if err != nil {
		s.log.Error("failed to create GET /api/schedule worker", err.Error(), "user", user.Name)
	}
	go func() {
		scheduleWorker.Process(childCtx)
	}()
	s.log.Info("Schedule worker started", "user", user.Name)

	// Start worker to buy tickets
	s.log.Info("Ticket scenario preparing", "user", user.Name)
	ticketScenarioWorker, err := worker.NewWorker(func(childCtx context.Context, _ int) {
		s.runBuyTicketScenario(childCtx, agent, user)
	}, worker.WithLoopCount(1), worker.WithMaxParallelism(1))
	if err != nil {
		s.log.Error("failed to create GET /api/schedule worker", err.Error(), "user", user.Name)
	}
	go func() {
		ticketScenarioWorker.Process(childCtx)
	}()
	s.log.Info("Ticket scenario started", "user", user.Name)

	s.checkSession(ctx, agent, user)

  s.log.Info("user", user.Name, "Session ended", "user", user.Name)
}

func (s *Scenario) runBuyTicketScenario(ctx context.Context, agent *agent.Agent, user User) error {
	s.sendInitRequests(ctx, agent, user)

	resp, err := HttpGet(ctx, agent, "/api/schedules")
	if err != nil {
		s.log.Error("failed to get /api/schedules", err.Error(), "user", user.Name)
	}
	s.log.Info("GET /api/schedules", "statusCode", resp.StatusCode, "user", user.Name)

	var schedules TrainScheduleResp
	if err := json.Unmarshal(resp.Body, &schedules); err != nil {
		return err
	}

	return nil
}

func (s *Scenario) sendInitRequests(ctx context.Context, agent *agent.Agent, user User) {
	resp, err := HttpGet(ctx, agent, "/api/purchased_tickets")
	if err != nil {
		s.log.Error("failed to get /api/purchased_tickets", err.Error(), "user", user.Name)
	}
	s.log.Info("GET /api/purchased_tickets", "statusCode", resp.StatusCode, "user", user.Name)

	resp, err = HttpGet(ctx, agent, "/api/stations")
	if err != nil {
		s.log.Error("failed to get /api/stations", err.Error(), "user", user.Name)
	}
	s.log.Info("GET /api/stations", "statusCode", resp.StatusCode, "user", user.Name)

	resp, err = HttpGet(ctx, agent, "/api/current_time")
	if err != nil {
		s.log.Error("failed to get /api/current_time", err.Error(), "user", user.Name)
	}
	s.log.Info("GET /api/current_time", "statusCode", resp.StatusCode, "user", user.Name)
}


func (s *Scenario) waitInWaitingRoom(ctx context.Context, agent *agent.Agent, user User) error {
	for {
		resp, err := HttpGet(ctx, agent, "/api/waiting_status")
		if err != nil {
			return err
		}

		var waitingStatus WaitingStatusResp
		if err := json.Unmarshal(resp.Body, &waitingStatus); err != nil {
			return err
		}

		s.log.Info("GET /api/waiting_status", "status", waitingStatus.Status, "next_check", waitingStatus.NextCheck, "user", user.Name)

		if waitingStatus.Status == "ready" {
			break
		} else if waitingStatus.Status == "waiting" {
			time.Sleep(time.Duration(waitingStatus.NextCheck) * time.Millisecond)
		} else {
			s.log.Error("Unknown status",  waitingStatus.Status, "Stopping requests.", "user", user.Name)
			break
		}
	}
	return nil
}

func (s *Scenario) checkSession(ctx context.Context, agent *agent.Agent, user User) error {
	for {
		resp, err := HttpGet(ctx, agent, "/api/session")
		if err != nil {
			return err
		}

		var session SessionResp
		if err := json.Unmarshal(resp.Body, &session); err != nil {
			return err
		}

		s.log.Info("GET /api/session", "status", session.Status, "next_check", session.NextCheck, "user", user.Name)

		// Check status
		if session.Status == "session_expired" {
			s.log.Info("Session expired. Stopping requests.", "user", user.Name)
			break
		} else if session.Status == "active" {
			// Wait next_check milliseconds before next request
			time.Sleep(time.Duration(session.NextCheck) * time.Millisecond)
		} else {
			// Unknown status, decide what to do:
			s.log.Info("Unknown status", session.Status, "Stopping requests.", "user", user.Name)
			break
		}
	}
	return nil
}


func (s *Scenario) getRandomUser() (User, error) {
		csvFilePath := "./bench/data/users.csv"
		userTotalCount := 1001

    file, err := os.Open(csvFilePath)
    if err != nil {
        return User{}, fmt.Errorf("failed to open CSV file: %w", err)
    }
    defer file.Close()

    reader := csv.NewReader(file)
    reader.TrimLeadingSpace = true

    // Read the header line
    headers, err := reader.Read()
    if err != nil {
        return User{}, fmt.Errorf("failed to read CSV headers: %w", err)
    }

    // Map headers to their indices for flexibility
    headerMap := make(map[string]int)
    for idx, header := range headers {
        headerMap[header] = idx
    }

    // Ensure required headers are present
    requiredHeaders := []string{"name", "password", "global_payment_token"}
    for _, header := range requiredHeaders {
        if _, exists := headerMap[header]; !exists {
            return User{}, fmt.Errorf("missing required header: %s", header)
        }
    }

    var selectedUser User
		var count int = 0
    index := rand.Intn(userTotalCount)

    for {
        record, err := reader.Read()
        if err != nil {
            if errors.Is(err, os.ErrClosed) || errors.Is(err, io.EOF) {
                break
            }
            return User{}, fmt.Errorf("error reading CSV record: %w", err)
        }

        count++

				if count == index {
					selectedUser = User{
							Name:               record[headerMap["name"]],
							Password:           record[headerMap["password"]],
							GlobalPaymentToken: record[headerMap["global_payment_token"]],
					}
					break
				}
    }

    return selectedUser, nil
}
