package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"runtime"
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
	addWorkersFn            func(ticketPhase, salesPhase int32)
	purchasedSeats          *sync.Map // key: "scheduleId|seat", value: true
}

type InitializeResponse struct {
	InitializedAt time.Time `json:"initialized_at"`
	AppLanguage   string    `json:"app_language"`
}

func Run(targetURL string, logLevel string) {
	// Limit to 4 CPU cores for benchmark consistency
	runtime.GOMAXPROCS(4)

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
	var purchasedSeats sync.Map

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
		purchasedSeats:          &purchasedSeats,
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
	ticketPhaseWorkerCounts := []int{5, 5, 10, 20, 20, 20}

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

	// Define the function to add workers synchronously
	var lastTicketPhase atomic.Int32
	var lastSalesPhase atomic.Int32

	addWorkersFn := func(newTicketPhase, newSalesPhase int32) {
		oldTicketPhase := lastTicketPhase.Load()
		oldSalesPhase := lastSalesPhase.Load()

		// Handle ticket phase change
		if newTicketPhase > oldTicketPhase {
			for phase := oldTicketPhase + 1; phase <= newTicketPhase; phase++ {
				addedWorkers := ticketPhaseWorkerCounts[phase]
				worker.AddParallelism(int32(addedWorkers))

				currentTimeStr := getApplicationClock(scenario.initializedAt)
				log.Info("New ad campaign launched!",
					"ticket_phase", fmt.Sprintf("%d/%d", phase, len(ticketSoldPhases)),
					"new_buyers", addedWorkers,
					"current_time", currentTimeStr,
					"user", "admin",
				)
			}
			lastTicketPhase.Store(newTicketPhase)
		}

		// Handle sales phase change
		if newSalesPhase > oldSalesPhase {
			for phase := oldSalesPhase + 1; phase <= newSalesPhase; phase++ {
				addedWorkers := salesPhaseWorkerCounts[phase]
				worker.AddParallelism(int32(addedWorkers))
				currentTimeStr := getApplicationClock(scenario.initializedAt)
				log.Info("New ad campaign launched!",
					"sales_phase", fmt.Sprintf("%d/%d", phase, len(salesPhases)),
					"new_buyers", addedWorkers,
					"current_time", currentTimeStr,
					"user", "admin",
				)
			}
			lastSalesPhase.Store(newSalesPhase)
		}
	}

	scenario.addWorkersFn = addWorkersFn

	// Run worker in a goroutine so we can handle critical errors
	workerDone := make(chan struct{})
	go func() {
		worker.Process(ctx)
		close(workerDone)
	}()

	// Wait for either worker completion or critical error
	var criticalErrorMessage string
	select {
	case <-workerDone:
		// Normal completion
	case critErr := <-criticalError:
		// Critical error occurred, cancel context and stop benchmark
		criticalErrorMessage = critErr.Error()
		slog.Error("Critical error occurred, stopping benchmark", "error", criticalErrorMessage)
		cancel()
		// Wait a bit for goroutines to clean up
		<-workerDone
	}

	currentTimeStr = getApplicationClock(scenario.initializedAt)
	finalSales := totalSales.Load()
	finalPurchased := totalPurchased.Load()
	finalTickets := totalTickets.Load()
	finalTicketPhase := currentTicketPhaseIndex.Load()
	finalSalesPhase := currentSalesPhaseIndex.Load()
	fmt.Printf("\nMain phase finished. Waiting for pending refunds to complete... (current_time: %s)\n", currentTimeStr)

	// Wait for all refund operations to complete with timeout
	refundDone := make(chan struct{})
	go func() {
		refundWg.Wait()
		close(refundDone)
	}()

	refundTimeout := false
	select {
	case <-refundDone:
		// Refunds completed successfully
	case <-time.After(10 * time.Second):
		slog.Error("Refund operations timed out after 10 seconds")
		criticalErrorMessage = "Refund operations timed out"
		refundTimeout = true
	}

	// Check if there was a critical error during refund phase
	if !refundTimeout {
		select {
		case critErr := <-criticalError:
			criticalErrorMessage = critErr.Error()
			slog.Error("Critical error occurred, stopping benchmark", "error", criticalErrorMessage)
			panic(critErr)
		default:
			// No critical error, proceed with final score
		}
	}

	finalRefunds := totalRefunds.Load()
	score := int64((float64(finalSales) + float64(finalPurchased-finalSales)*0.5 - float64(finalRefunds)) / 100)

	// Set score to 0 if refund timeout occurred
	if refundTimeout {
		score = 0
	}

	time.Sleep(3 * time.Second) // Wait for slog to flush

	// Validate no duplicate seats were purchased
	totalSeats := int64(0)
	scenario.purchasedSeats.Range(func(key, value interface{}) bool {
		totalSeats++
		return true
	})

	duplicateSeatsFound := totalSeats < finalTickets
	if criticalErrorMessage == "" && duplicateSeatsFound {
		slog.Error("Duplicate seat assignments detected!", "total_purchased_seats", finalTickets, "unique_seats", totalSeats)
		criticalErrorMessage = "Duplicate seat assignments detected"
		score = 0
	}

	// Always output final results regardless of log level
	fmt.Println("\nBenchmark Finished!")
	if criticalErrorMessage != "" {
		fmt.Println("  Interrupted due to critical error:")
		fmt.Printf("  %s\n\n", criticalErrorMessage)
	}

	fmt.Printf("  Score: %d\n", score)
	fmt.Printf("  Total Sales: %d\n", finalSales)
	fmt.Printf("  Total Purchased: %d\n", finalPurchased)
	fmt.Printf("  Total Refunds: %d\n", finalRefunds)
	fmt.Printf("  Net Revenue: %d\n", finalSales-finalRefunds)
	fmt.Printf("  Total Tickets: %d\n", finalTickets)
	fmt.Printf("  Ticket Phase: %d/%d\n", finalTicketPhase, len(ticketSoldPhases))
	fmt.Printf("  Sales Phase: %d/%d\n", finalSalesPhase, len(salesPhases))
	fmt.Printf("  Current Time: %s\n\n", currentTimeStr)

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
