package bench

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/showwin/ISHOCON3/benchmark/bench/data"

	"github.com/isucon/isucandar/agent"
)

type AdminStatsResponse struct {
	TotalSales   int64 `json:"total_sales"`
	TotalRefunds int64 `json:"total_refunds"`
}

type TrainSalesData struct {
	TrainName        string `json:"train_name"`
	TicketsSold      int64  `json:"tickets_sold"`
	PendingRevenue   int64  `json:"pending_revenue"`
	ConfirmedRevenue int64  `json:"confirmed_revenue"`
	Refunds          int64  `json:"refunds"`
}

type TrainSalesResponse struct {
	Trains []TrainSalesData `json:"trains"`
}

type TrainModelsResponse struct {
	ModelNames []string `json:"model_names"`
}

type AddTrainRequest struct {
	TrainName      string   `json:"train_name"`
	ModelName      string   `json:"model_name"`
	DepartureTimes []string `json:"departure_times"`
}

type AddTrainResponse struct {
	Status string `json:"status"`
}

type TrainConfig struct {
	ModelName          string
	NamePrefix         string
	FirstDepartureTime string
}

type RegistrationPhase struct {
	Threshold  int64
	TrainCount int
}

// 12 trains in total
var ticketSoldPhases = []RegistrationPhase{
	{Threshold: 5, TrainCount: 1},
	{Threshold: 10, TrainCount: 2},
	{Threshold: 50, TrainCount: 3},
	{Threshold: 100, TrainCount: 3},
	{Threshold: 200, TrainCount: 3},
}

// 68 trains in total
var salesPhases = []RegistrationPhase{
	{Threshold: 1000, TrainCount: 3},
	{Threshold: 3000, TrainCount: 3},
	{Threshold: 10000, TrainCount: 5},
	{Threshold: 50000, TrainCount: 7},
	{Threshold: 200000, TrainCount: 10},
	{Threshold: 500000, TrainCount: 20},
	{Threshold: 1000000, TrainCount: 20},
}

func (s *Scenario) RunAdminScenario(ctx context.Context) {
	agent, err := agent.NewAgent(agent.WithBaseURL(s.targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
	if err != nil {
		s.log.Error("Failed to create agent", "error", err.Error())
		return
	}

	// For safer staging after initialization
	time.Sleep(1 * time.Second)

	s.log.Info("Admin scenario started")

	// The first check is at 00:40, so start after 4 seconds
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("Admin scenario finished")
			return
		case <-ticker.C:
			err := s.adminLogin(ctx, agent)
			if err != nil {
				s.criticalError <- fmt.Errorf("failed to login as admin: %w", err)
				return
			}
			s.log.Info("POST /api/login", "user", "admin")

			_, err = s.getTrainModels(ctx, agent)
			if err != nil {
				s.criticalError <- fmt.Errorf("failed to get train models: %w", err)
				return
			}
			s.log.Info("GET /api/train_models", "user", "admin")

			// Record current benchmark values before waiting
			minExpectedSales := s.totalSales.Load()
			minExpectedRefunds := s.totalRefunds.Load()
			minExpectedTickets := s.totalTickets.Load()

			// Wait 1 second to allow admin page to catch up with latest data
			time.Sleep(1 * time.Second)

			// Call GET /api/admin/stats with 2 second timeout
			statsCtx, statsCancel := context.WithTimeout(ctx, 2*time.Second)
			stats, err := s.getAdminStats(statsCtx, agent)
			statsCancel()
			if err != nil {
				s.criticalError <- fmt.Errorf("failed to get admin stats within 2 second: %w", err)
				return
			}
			s.log.Info("GET /api/admin/stats", "user", "admin")

			// Call GET /api/admin/train_sales with 2 second timeout
			trainSalesCtx, trainSalesCancel := context.WithTimeout(ctx, 2*time.Second)
			trainSales, err := s.getAdminTrainSales(trainSalesCtx, agent)
			trainSalesCancel()
			if err != nil {
				s.criticalError <- fmt.Errorf("failed to get train sales within 2 second: %w", err)
				return
			}
			s.log.Info("GET /api/admin/train_sales", "user", "admin")

			// Buffer before validations
			time.Sleep(200 * time.Millisecond)

			// Fetch current benchmark values after waiting
			maxExpectedSales := s.totalSales.Load()
			maxExpectedRefunds := s.totalRefunds.Load()
			maxExpectedTickets := s.totalTickets.Load()

			// Validate stats: API values should be >= min expected and <= max expected
			if stats.TotalSales < minExpectedSales {
				err := fmt.Errorf("total_sales too old: API returned %d, but minimum expected is %d",
					stats.TotalSales, minExpectedSales)
				s.log.Error("Stats validation failed", "error", err.Error(), "user", "admin")
				s.criticalError <- err
				return
			}
			if stats.TotalSales > maxExpectedSales {
				err := fmt.Errorf("total_sales too large: API returned %d, but maximum expected is %d",
					stats.TotalSales, maxExpectedSales)
				s.log.Error("Stats validation failed", "error", err.Error(), "user", "admin")
				s.criticalError <- err
				return
			}

			if stats.TotalRefunds < minExpectedRefunds {
				err := fmt.Errorf("total_refunds too old: API returned %d, but minimum expected is %d",
					stats.TotalRefunds, minExpectedRefunds)
				s.log.Error("Stats validation failed", "error", err.Error(), "user", "admin")
				s.criticalError <- err
				return
			}
			if stats.TotalRefunds > maxExpectedRefunds {
				err := fmt.Errorf("total_refunds too large: API returned %d, but maximum expected is %d",
					stats.TotalRefunds, maxExpectedRefunds)
				s.log.Error("Stats validation failed", "error", err.Error(), "user", "admin")
				s.criticalError <- err
				return
			}

			s.log.Info("Stats validation passed", "total_sales", stats.TotalSales, "total_refunds", stats.TotalRefunds, "user", "admin")

			// Validate total tickets sold
			var totalTicketsSold int64
			for _, train := range trainSales.Trains {
				totalTicketsSold += train.TicketsSold
			}

			if totalTicketsSold < minExpectedTickets {
				err := fmt.Errorf("total_tickets_sold too old: API returned %d, but minimum expected is %d",
					totalTicketsSold, minExpectedTickets)
				s.log.Error("Tickets validation failed", "error", err.Error(), "user", "admin")
				s.criticalError <- err
				return
			}
			if totalTicketsSold > maxExpectedTickets {
				err := fmt.Errorf("total_tickets_sold too large: API returned %d, but maximum expected is %d",
					totalTicketsSold, maxExpectedTickets)
				s.log.Error("Tickets validation failed", "error", err.Error(), "user", "admin")
				s.criticalError <- err
				return
			}

			s.log.Info("Tickets validation passed", "total_tickets_sold", totalTicketsSold, "user", "admin")
			s.log.Info("Thinking whether to add new trains", "user", "admin")

			// Register more trains based on tickets and sales
			err = s.registerNewTrains(ctx, agent, maxExpectedTickets, maxExpectedSales)

			if err != nil {
				s.log.Error("Failed to register trains", "error", err.Error(), "user", "admin")
				s.criticalError <- fmt.Errorf("train registration failed: %w", err)
				return
			}
		}
	}
}

// registerNewTrains checks if thresholds are exceeded and registers trains accordingly
func (s *Scenario) registerNewTrains(ctx context.Context, agent *agent.Agent, currentTickets, currentSales int64) error {
	// Check ticket sold
	currentTicketPhase := int(s.currentTicketPhaseIndex.Load())
	for i := currentTicketPhase; i < len(ticketSoldPhases); i++ {
		phase := ticketSoldPhases[i]
		if currentTickets >= phase.Threshold {
			s.log.Info("Registering new trains based on ticket sold",
				"current_phase", i+1,
				"threshold", phase.Threshold,
				"current_tickets", currentTickets,
				"new_trains", phase.TrainCount,
				"user", "admin")

			// Calculate how many trains were already registered
			var alreadyRegistered int
			for j := 0; j < i; j++ {
				alreadyRegistered += ticketSoldPhases[j].TrainCount
			}

			csvStart := alreadyRegistered
			csvCount := phase.TrainCount

			err := s.registerTrainsFromCSV(ctx, agent, "ticket_sold", csvStart, csvCount)
			if err != nil {
				return fmt.Errorf("failed to register trains for ticket phase %d: %w", i, err)
			}

			// Move to next phase
			s.currentTicketPhaseIndex.Store(int32(i + 1))
		} else {
			s.log.Info("Not enough tickets sold for the next phase", "current_phase", i, "user", "admin")
			break
		}
	}

	// Check sales
	currentSalesPhase := int(s.currentSalesPhaseIndex.Load())
	for i := currentSalesPhase; i < len(salesPhases); i++ {
		phase := salesPhases[i]
		if currentSales >= phase.Threshold {
			s.log.Info("Registering new trains based on sales",
				"phase", i+1,
				"threshold", phase.Threshold,
				"current_sales", currentSales,
				"new_trains", phase.TrainCount,
				"user", "admin")

			// Calculate how many trains were already registered
			var alreadyRegistered int
			for j := 0; j < i; j++ {
				alreadyRegistered += salesPhases[j].TrainCount
			}

			csvStart := alreadyRegistered
			csvCount := phase.TrainCount

			err := s.registerTrainsFromCSV(ctx, agent, "sales", csvStart, csvCount)
			if err != nil {
				return fmt.Errorf("failed to register trains for sales phase %d: %w", i, err)
			}

			// Move to next phase
			s.currentSalesPhaseIndex.Store(int32(i + 1))
		} else {
			s.log.Info("Not enough sales for the next phase", "current_phase", i, "user", "admin")
			break
		}
	}

	return nil
}

// registerTrainsFromCSV reads configs from CSV starting at a given index and registers trains
func (s *Scenario) registerTrainsFromCSV(ctx context.Context, agent *agent.Agent, csvType string, startIndex, count int) error {
	allConfigs, err := readAllTrainConfigs(csvType)
	if err != nil {
		s.log.Error("Failed to read all train configs", "csv_type", csvType, "error", err.Error(), "user", "admin")
		return fmt.Errorf("failed to read all train configs: %w", err)
	}

	endIndex := startIndex + count
	if endIndex > len(allConfigs) {
		s.log.Error("Not enough train configs in CSV", "csv_type", csvType, "need_end_index", endIndex, "have", len(allConfigs), "user", "admin")
		return fmt.Errorf("not enough train configs in CSV: need %d-%d, but only have %d", startIndex, endIndex, len(allConfigs))
	}

	configs := allConfigs[startIndex:endIndex]

	// Register each train
	for _, config := range configs {
		trainName := generateTrainName(config.ModelName, config.NamePrefix)

		departureTimes, err := generateDepartureTimes(config.FirstDepartureTime)
		if err != nil {
			s.log.Error("Failed to generate departure times", "error", err.Error(), "user", "admin")
			return fmt.Errorf("failed to generate departure times: %w", err)
		}

		req := AddTrainRequest{
			TrainName:      trainName,
			ModelName:      config.ModelName,
			DepartureTimes: departureTimes,
		}

		err = s.addTrain(ctx, agent, req)
		if err != nil {
			s.log.Error("addTrain returned error", "error", err.Error(), "user", "admin")
			return fmt.Errorf("failed to add train %s: %w", trainName, err)
		}
		s.log.Info("POST /api/admin/add_train", "status", 200, "train_name", trainName, "model", config.ModelName, "user", "admin")
	}

	return nil
}

func generateTrainName(modelName string, namePrefix string) string {
	// Business-4 -> B4
	parts := strings.Split(modelName, "-")
	for i := range parts {
		parts[i] = string(parts[i][0])
	}
	modelPrefix := strings.Join(parts, "")
	randomDigit := rand.Intn(10)
	return fmt.Sprintf("%s%s%d", modelPrefix, namePrefix, randomDigit)
}

// generateDepartureTimes generates departure times every 3 hours starting from firstTime
// For example: 01:00 -> [01:00, 04:00, 07:00, 10:00, 13:00, 16:00, 19:00, 22:00]
func generateDepartureTimes(firstTime string) ([]string, error) {
	startTime, err := time.Parse("15:04", firstTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse time %s: %w", firstTime, err)
	}

	// Get starting hour
	startHour, _ := strconv.Atoi(startTime.Format("15"))

	times := []string{firstTime}
	currentHour := startHour

	for {
		currentHour += 3

		// Stop if we reach or exceed 24:00
		if currentHour >= 24 {
			break
		}

		times = append(times, fmt.Sprintf("%02d:%s", currentHour, startTime.Format("04")))
	}

	return times, nil
}

func readAllTrainConfigs(csvType string) ([]TrainConfig, error) {
	// Use embedded CSV data instead of file system
	var csvData string
	switch csvType {
	case "ticket_sold":
		csvData = data.TrainConfigsTicketSoldCSV
	case "sales":
		csvData = data.TrainConfigsSalesCSV
	default:
		return nil, fmt.Errorf("unknown CSV type: %s", csvType)
	}

	reader := csv.NewReader(bytes.NewReader([]byte(csvData)))
	reader.TrimLeadingSpace = true

	// Exclude header
	_, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	var configs []TrainConfig
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to read record: %w", err)
		}

		if len(record) < 3 {
			return nil, fmt.Errorf("invalid record format: %v", record)
		}

		configs = append(configs, TrainConfig{
			ModelName:          record[0],
			NamePrefix:         record[1],
			FirstDepartureTime: record[2],
		})
	}

	return configs, nil
}

// API call helpers for admin scenario

func (s *Scenario) adminLogin(ctx context.Context, agent *agent.Agent) error {
	reqBody := &LoginReq{
		Name:     "admin",
		Password: "admin",
	}
	reqBodyBuf, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal login request: %w", err)
	}

	resp, err := HttpPost(ctx, agent, "/api/login", bytes.NewReader(reqBodyBuf))
	if err != nil {
		return fmt.Errorf("failed to post /api/login: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("login failed with status code %d", resp.StatusCode)
	}

	s.log.Info("Admin logged in successfully")
	return nil
}

func (s *Scenario) getTrainModels(ctx context.Context, agent *agent.Agent) (*TrainModelsResponse, error) {
	resp, err := HttpGet(ctx, agent, "/api/train_models")
	if err != nil {
		return nil, fmt.Errorf("failed to get /api/train_models: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("get train models failed with status code %d", resp.StatusCode)
	}

	var trainModels TrainModelsResponse
	if err := json.Unmarshal(resp.Body, &trainModels); err != nil {
		return nil, fmt.Errorf("failed to unmarshal train models response: %w", err)
	}

	return &trainModels, nil
}

func (s *Scenario) getAdminStats(ctx context.Context, agent *agent.Agent) (*AdminStatsResponse, error) {
	resp, err := HttpGet(ctx, agent, "/api/admin/stats")
	if err != nil {
		return nil, fmt.Errorf("failed to get /api/admin/stats: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("get admin stats failed with status code %d", resp.StatusCode)
	}

	var stats AdminStatsResponse
	if err := json.Unmarshal(resp.Body, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stats response: %w", err)
	}

	return &stats, nil
}

func (s *Scenario) getAdminTrainSales(ctx context.Context, agent *agent.Agent) (*TrainSalesResponse, error) {
	resp, err := HttpGet(ctx, agent, "/api/admin/train_sales")
	if err != nil {
		return nil, fmt.Errorf("failed to get /api/admin/train_sales: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("get train sales failed with status code %d", resp.StatusCode)
	}

	var trainSales TrainSalesResponse
	if err := json.Unmarshal(resp.Body, &trainSales); err != nil {
		return nil, fmt.Errorf("failed to unmarshal train sales response: %w", err)
	}

	return &trainSales, nil
}

func (s *Scenario) addTrain(ctx context.Context, agent *agent.Agent, req AddTrainRequest) error {
	reqBodyBuf, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal add train request: %w", err)
	}

	resp, err := HttpPost(ctx, agent, "/api/admin/add_train", bytes.NewReader(reqBodyBuf))
	if err != nil {
		return fmt.Errorf("failed to post /api/admin/add_train: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("add train failed with status code %d", resp.StatusCode)
	}

	var addTrainResp AddTrainResponse
	if err := json.Unmarshal(resp.Body, &addTrainResp); err != nil {
		return fmt.Errorf("failed to unmarshal add train response: %w", err)
	}

	if addTrainResp.Status != "success" {
		return fmt.Errorf("add train failed with status: %s", addTrainResp.Status)
	}
	return nil
}
