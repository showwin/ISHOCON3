package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"github.com/showwin/ISHOCON3/benchmark/bench/logger"

	"github.com/isucon/isucandar/agent"
	"github.com/isucon/isucandar/worker"
)

var jst = time.FixedZone("Asia/Tokyo", 9*60*60)

type NewUserCount struct {
	Count int
}

type Scenario struct {
	targetURL               string
	initializedAt           time.Time
	appLanguage             string
	log                     logger.Logger
	totalSales              *atomic.Int64
	totalRefunds            *atomic.Int64
	totalPurchased          *atomic.Int64
	totalTickets            *atomic.Int64
	refundWg                *sync.WaitGroup
	criticalError           chan error
	currentTicketPhaseIndex *atomic.Int32
	currentSalesPhaseIndex  *atomic.Int32
}

type InitializeResponse struct {
	InitializedAt time.Time `json:"initialized_at"`
	AppLanguage   string    `json:"app_language"`
}

func Run(targetURL string, logLevel string) {
	rand.New(rand.NewSource(time.Now().UnixNano())) // Seed random number generator

	agent, err := agent.NewAgent(agent.WithBaseURL(targetURL), agent.WithTimeout(10*time.Second), agent.WithDefaultTransport())
	if err != nil {
		slog.Error("failed to create agent", "error", err.Error())
	}
	httpResp, err := HttpPost(context.Background(), agent, "/api/initialize", nil)
	if err != nil {
		slog.Error("failed to post /initialize", "error", err.Error())
	}
	var initResp InitializeResponse
	if err := json.Unmarshal(httpResp.Body, &initResp); err != nil {
		slog.Error("failed to unmarshal response", "error", err.Error())
	}
	if httpResp.StatusCode != 200 {
		slog.Error("initialize returned non-200 status", "status_code", httpResp.StatusCode, "body", string(httpResp.Body))
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Initialize atomic counters for sales and refunds
	var totalSales atomic.Int64
	var totalRefunds atomic.Int64
	var totalPurchased atomic.Int64
	var totalTickets atomic.Int64
	var currentTicketPhaseIndex atomic.Int32
	var currentSalesPhaseIndex atomic.Int32
	var refundWg sync.WaitGroup
	criticalError := make(chan error, 1) // Buffered channel to prevent blocking

	log := logger.GetLogger(logLevel)
	scenario := Scenario{
		targetURL:               targetURL,
		initializedAt:           initResp.InitializedAt,
		appLanguage:             initResp.AppLanguage,
		log:                     log,
		totalSales:              &totalSales,
		totalRefunds:            &totalRefunds,
		totalPurchased:          &totalPurchased,
		totalTickets:            &totalTickets,
		refundWg:                &refundWg,
		criticalError:           criticalError,
		currentTicketPhaseIndex: &currentTicketPhaseIndex,
		currentSalesPhaseIndex:  &currentSalesPhaseIndex,
	}

	currentTimeStr := getApplicationClock(scenario.initializedAt)
	slog.Info("Benchmark Start!", "current_time", currentTimeStr)

	// Start admin scenario
	go scenario.RunAdminScenario(ctx)

	// Define worker counts per ticket phase.
	// <tickets> => <added workers>
	// 0 => 5
	// 5 => +5
	// 10 => +10
	// 50 => +20
	// 100 => +20
	// 200 => +20
	ticketPhaseWorkerCounts := []int{5, 10, 20, 20, 20}

	// Define worker counts per sales phase.
	// <sales> => <added workers>
	// 0 => 10
	// 1000 => +5
	// 3000 => +5
	// 10000 => +5
	// 50000 => +20
	// 200000 => +50
	// 500000 => +100
	// 1000000 => +100
	salesPhaseWorkerCounts := []int{15, 5, 5, 5, 20, 50, 100, 100}

	worker, err := worker.NewWorker(func(ctx context.Context, _ int) {
		// Run the user scenario
		scenario.RunUserScenario(ctx)
	}, worker.WithInfinityLoop(), worker.WithMaxParallelism(int32(salesPhaseWorkerCounts[0]+ticketPhaseWorkerCounts[0])))
	if err != nil {
		panic(err)
	}

	// Monitor phase changes and adjust worker parallelism dynamically
	go func() {
		lastTicketPhase := int32(0)
		lastSalesPhase := int32(0)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				currentTicketPhase := scenario.currentTicketPhaseIndex.Load()
				currentSalesPhase := scenario.currentSalesPhaseIndex.Load()

				// Handle ticket phase change
				if currentTicketPhase != lastTicketPhase && currentTicketPhase > 0 && int(currentTicketPhase) < len(ticketPhaseWorkerCounts) {
					addedWorkers := ticketPhaseWorkerCounts[currentTicketPhase]
					worker.AddParallelism(int32(addedWorkers))

					currentTimeStr := getApplicationClock(scenario.initializedAt)
					log.Info("New ad campaign launched!",
						"ticket_phase", fmt.Sprintf("%d/%d", currentTicketPhase, len(ticketSoldPhases)),
						"new_buyers", addedWorkers,
						"current_time", currentTimeStr,
						"user", "admin",
					)
					lastTicketPhase = currentTicketPhase
				}

				// Handle sales phase change
				if currentSalesPhase != lastSalesPhase && currentSalesPhase > 0 && int(currentSalesPhase) < len(salesPhaseWorkerCounts) {
					addedWorkers := salesPhaseWorkerCounts[currentSalesPhase]
					worker.AddParallelism(int32(addedWorkers))

					// Log ad campaign launch
					currentTimeStr := getApplicationClock(scenario.initializedAt)
					log.Info("New ad campaign launched!",
						"sales_phase", fmt.Sprintf("%d/%d", currentSalesPhase, len(salesPhases)),
						"new_buyers", addedWorkers,
						"current_time", currentTimeStr,
						"user", "admin",
					)
					lastSalesPhase = currentSalesPhase
				}
			}
		}
	}()

	// Run worker in a goroutine so we can handle critical errors
	workerDone := make(chan struct{})
	go func() {
		worker.Process(ctx)
		close(workerDone)
	}()

	// Wait for either worker completion or critical error
	select {
	case <-workerDone:
		// Normal completion
	case critErr := <-criticalError:
		// Critical error occurred, cancel context and stop benchmark
		slog.Error("Critical error occurred, stopping benchmark", "error", critErr.Error())
		cancel()
		// Wait a bit for goroutines to clean up
		<-workerDone
	}

	currentTimeStr = getApplicationClock(scenario.initializedAt)
	slog.Info("Main phase finished. Waiting for pending refunds to complete...", "current_time", currentTimeStr)

	// Wait for all refund operations to complete
	refundWg.Wait()

	// Check if there was a critical error during refund phase
	select {
	case critErr := <-criticalError:
		slog.Error("Critical error occurred, stopping benchmark", "error", critErr.Error())
		panic(critErr)
	default:
		// No critical error, proceed with final score
	}

	finalSales := totalSales.Load()
	finalRefunds := totalRefunds.Load()
	finalPurchased := totalPurchased.Load()
	finalTickets := totalTickets.Load()
	finalTicketPhase := currentTicketPhaseIndex.Load()
	finalSalesPhase := currentSalesPhaseIndex.Load()
	currentTimeStr = getApplicationClock(scenario.initializedAt)

	score := int64((float64(finalSales) + float64(finalPurchased-finalSales)*0.5 - float64(finalRefunds)) / 100)

	time.Sleep(2 * time.Second) // Wait for slog to flush

	slog.Info("Benchmark Finished!",
		"score", score,
		"total_sales", finalSales,
		"total_purchased", finalPurchased,
		"total_refunds", finalRefunds,
		"net_revenue", finalSales-finalRefunds,
		"total_tickets", finalTickets,
		"ticket_phase", fmt.Sprintf("%d/%d", finalTicketPhase, len(ticketSoldPhases)),
		"sales_phase", fmt.Sprintf("%d/%d", finalSalesPhase, len(salesPhases)),
		"current_time", currentTimeStr)
	postScore(score, scenario.appLanguage)
}

func postScore(score int64, appLanguage string) {
	apiURL := os.Getenv("BENCH_SCOREBOARD_APIGW_URL")
	teamName := os.Getenv("BENCH_TEAM_NAME")
	if apiURL == "" && teamName == "" {
		return
	}

	location, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		slog.Error("Failed to send score")
		slog.Error("Error loading location.", "error", err.Error())
		return
	}
	now := time.Now().In(location)
	timestamp := now.Format(time.RFC3339)

	data := map[string]interface{}{
		"team":      teamName,
		"score":     score,
		"timestamp": timestamp,
		"language":  appLanguage,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		slog.Error("Failed to send score")
		slog.Error("Error encoding JSON.", "error", err.Error())
		return
	}

	// Create the PUT request
	req, err := http.NewRequest("PUT", apiURL+"teams", bytes.NewBuffer(jsonData))
	if err != nil {
		slog.Error("Failed to send score")
		slog.Error("Error creating request.", "error", err.Error())
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request using the http.DefaultClient
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to send score")
		slog.Error("Error sending request.", "error", err.Error())
		return
	}
	defer resp.Body.Close()
	slog.Info("Score sent to scoreboard")
}
